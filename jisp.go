// Jisp is a stack-based programming system designed for simplicity and integration.
// It uses JSON as its underlying data model for code, stack, and variables,
// making it highly debuggable and interoperable with other tools.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// Operation represents a single JSON Patch operation.
type Operation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// Patch is a slice of Operations.
type Patch []Operation

// CreatePatch generates a JSON Patch to transform the 'before' document into the 'after' document.
func CreatePatch(before, after []byte) (Patch, error) {
	var beforeData, afterData interface{}
	if err := json.Unmarshal(before, &beforeData); err != nil {
		return nil, fmt.Errorf("error unmarshaling before document: %w", err)
	}
	if err := json.Unmarshal(after, &afterData); err != nil {
		return nil, fmt.Errorf("error unmarshaling after document: %w", err)
	}

	return diff("", beforeData, afterData), nil
}

func diff(path string, before, after interface{}) Patch {
	var patch Patch

	if !reflect.DeepEqual(before, after) {
		beforeMap, beforeIsMap := before.(map[string]interface{})
		afterMap, afterIsMap := after.(map[string]interface{})

		if beforeIsMap && afterIsMap {
			for key, valBefore := range beforeMap {
				currentPath := path + "/" + key
				if valAfter, ok := afterMap[key]; ok {
					patch = append(patch, diff(currentPath, valBefore, valAfter)...)
				} else {
					patch = append(patch, Operation{Op: "remove", Path: currentPath})
				}
			}
			for key, valAfter := range afterMap {
				currentPath := path + "/" + key
				if _, ok := beforeMap[key]; !ok {
					patch = append(patch, Operation{Op: "add", Path: currentPath, Value: valAfter})
				}
			}
		} else {
			patch = append(patch, Operation{Op: "replace", Path: path, Value: after})
		}
	}
	return patch
}

func (p Patch) Apply(doc []byte) ([]byte, error) {
	var data interface{}
	if err := json.Unmarshal(doc, &data); err != nil {
		return nil, fmt.Errorf("error unmarshaling document: %w", err)
	}

	for _, op := range p {
		if err := applyOp(&data, op); err != nil {
			return nil, fmt.Errorf("error applying operation %v: %w", op, err)
		}
	}

	return json.Marshal(data)
}

func applyOp(doc *interface{}, op Operation) error {
	parts := strings.Split(op.Path, "/")[1:]

	container, key, err := findContainer(*doc, parts)
	if err != nil {
		return err
	}

	switch op.Op {
	case "add", "replace":
		m, ok := container.(map[string]interface{})
		if !ok {
			return fmt.Errorf("container is not a map for path %s", op.Path)
		}
		m[key] = op.Value
	case "remove":
		m, ok := container.(map[string]interface{})
		if !ok {
			return fmt.Errorf("container is not a map for path %s", op.Path)
		}
		delete(m, key)
	default:
		return fmt.Errorf("unsupported operation: %s", op.Op)
	}
	return nil
}

func findContainer(doc interface{}, pathParts []string) (interface{}, string, error) {
	if len(pathParts) == 0 {
		return nil, "", fmt.Errorf("empty path")
	}

	current := doc
	for i := 0; i < len(pathParts)-1; i++ {
		part := pathParts[i]
		switch c := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = c[part]
			if !ok {
				return nil, "", fmt.Errorf("path not found: %s", part)
			}
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil {
				return nil, "", fmt.Errorf("invalid array index: %s", part)
			}
			if idx < 0 || idx >= len(c) {
				return nil, "", fmt.Errorf("index out of bounds: %d", idx)
			}
			current = c[idx]
		default:
			return nil, "", fmt.Errorf("invalid path segment %s", part)
		}
	}
	return current, pathParts[len(pathParts)-1], nil
}

func (p Patch) MarshalJSON() ([]byte, error) {
	return json.Marshal([]Operation(p))
}

func DecodePatch(data []byte) (Patch, error) {
	var p Patch
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return p, nil
}

var (
	ErrBreak      = errors.New("break")
	ErrContinue   = errors.New("continue")
	ErrReturn     = errors.New("return")
	ErrExit       = errors.New("exit")
	ErrBreakpoint = errors.New("breakpoint")

	processManager = &ProcessManager{
		programs: make(map[string]*JispProgram),
	}
	pidCounter int
	pidMutex   sync.Mutex
)

// ProcessManager handles the lifecycle of spawned Jisp programs.
type ProcessManager struct {
	programs map[string]*JispProgram
	mutex    sync.RWMutex
}

func (pm *ProcessManager) Register(jp *JispProgram) string {
	pidMutex.Lock()
	pidCounter++
	pid := fmt.Sprintf("pid-%d", pidCounter)
	pidMutex.Unlock()

	jp.PID = pid
	pm.mutex.Lock()
	pm.programs[pid] = jp
	pm.mutex.Unlock()
	return pid
}

func (pm *ProcessManager) Get(pid string) (*JispProgram, bool) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	jp, ok := pm.programs[pid]
	return jp, ok
}

func exitOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) > 0 {
		return fmt.Errorf("exit error: expected 0 arguments, got %d", len(op.Args))
	}
	return ErrExit
}

type JispError struct {
	OperationName      string                 `json:"operation_name"`
	InstructionPointer []interface{}          `json:"instruction_pointer"`
	Message            string                 `json:"message"`
	StackSnapshot      []interface{}          `json:"stack_snapshot"`
	CallStackSnapshot  []*CallFrame           `json:"call_stack_snapshot"`
	VariablesSnapshot  map[string]interface{} `json:"variables_snapshot"`
}

func (e *JispError) Error() string {
	return fmt.Sprintf("Jisp error in '%s' at %v: %s", e.OperationName, e.InstructionPointer, e.Message)
}

func parseRawOperation(rawOp []interface{}) (JispOperation, error) {
	if len(rawOp) == 0 {
		return JispOperation{}, fmt.Errorf("operation array is empty")
	}

	opName, ok := rawOp[0].(string)
	if !ok {
		return JispOperation{}, fmt.Errorf("operation name is not a string, got %T", rawOp[0])
	}

	var args []interface{}
	if len(rawOp) > 1 {
		args = rawOp[1:]
	}

	return JispOperation{Name: opName, Args: args}, nil
}

// CallFrame represents a single frame on the call stack, holding the instruction
// pointer and the operations for its execution context.
type CallFrame struct {
	Ip        int                    `json:"-"`
	Ops       []JispOperation        `json:"Ops"`
	BasePath  []interface{}          `json:"BasePath"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

func (cf *CallFrame) MarshalJSON() ([]byte, error) {
	// To prevent recursion, we define a type alias that doesn't have MarshalJSON.
	type Alias CallFrame
	// We create a new struct for marshaling that has the desired 'Ip' type.
	return json.Marshal(&struct {
		Ip []interface{} `json:"Ip"`
		*Alias
	}{
		Ip:    append(cf.BasePath, cf.Ip),
		Alias: (*Alias)(cf),
	})
}

// Import defines a single import statement.
type Import struct {
	Path []string `json:"path,omitempty"`
	URL  string   `json:"url,omitempty"`
}

// JispProgram represents the entire state of a JISP program, including the
// execution stack, variables map, a general-purpose state map, and a call stack.
type JispProgram struct {
	PID          string                 `json:"pid,omitempty"`
	Stack        []interface{}          `json:"stack"`
	Variables    map[string]interface{} `json:"variables"`
	Imports      []Import               `json:"imports,omitempty"`
	Code         []JispOperation        `json:"code"`
	CallStack    []*CallFrame           `json:"call_stack"`
	Error        *JispError             `json:"error,omitempty"`
	History      []json.RawMessage      `json:"history"`
	SaveHistory  bool                   `json:"save_history,omitempty"`
	Debug        bool                   `json:"debug,omitempty"`
	Breakpoints  [][]interface{}        `json:"breakpoints,omitempty"`
	Running      bool                   `json:"running,omitempty"`
	done         chan struct{}          `json:"-"`
	runningMutex sync.Mutex             `json:"-"`
}

func (cf *CallFrame) UnmarshalJSON(data []byte) error {
	type Alias CallFrame
	aux := &struct {
		Ip interface{} `json:"Ip"`
		*Alias
	}{
		Alias: (*Alias)(cf),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	cf.BasePath = []interface{}{} // Initialize BasePath

	switch v := aux.Ip.(type) {
	case []interface{}:
		if len(v) == 0 {
			return fmt.Errorf("could not unmarshal Ip: instruction pointer path cannot be an empty array")
		}
		if ipFloat, ok := v[len(v)-1].(float64); ok {
			cf.Ip = int(ipFloat)
			if len(v) > 1 {
				cf.BasePath = v[:len(v)-1]
			}
		} else {
			return fmt.Errorf("could not unmarshal Ip: last element of path is not a number")
		}
	case nil:
		// Ip might be absent if the frame is not actively executing, which is fine.
		cf.Ip = 0
	default:
		return fmt.Errorf("could not unmarshal Ip: expected an array or null, got %T", aux.Ip)
	}

	return nil
}

func (jp *JispProgram) currentFrame() *CallFrame {
	if len(jp.CallStack) == 0 {
		return nil
	}
	return jp.CallStack[len(jp.CallStack)-1]
}

// newError creates a new JispError with the current program state.
func (jp *JispProgram) newError(op *JispOperation, message string) *JispError {
	stackCopy := make([]interface{}, len(jp.Stack))
	copy(stackCopy, jp.Stack)

	// Deep copy call stack
	callStackCopy := make([]*CallFrame, len(jp.CallStack))
	for i, frame := range jp.CallStack {
		// Note: This is a shallow copy of the frame's Ops and Variables.
		// For debugging snapshots, this is usually sufficient.
		frameCopy := *frame
		callStackCopy[i] = &frameCopy
	}

	// Deep copy variables
	varsCopy := make(map[string]interface{})
	for k, v := range jp.Variables {
		// Note: This is a shallow copy of nested structures.
		varsCopy[k] = v
	}

	return &JispError{
		OperationName:      op.Name,
		InstructionPointer: jp.currentInstructionPath(),
		Message:            message,
		StackSnapshot:      stackCopy,
		CallStackSnapshot:  callStackCopy,
		VariablesSnapshot:  varsCopy,
	}
}

// ensureInitialized checks if core JispProgram components (Stack, Variables, CallStack) are nil
// and initializes them to their zero values if so. This prevents nil pointer dereferences
// and ensures a consistent program state.
func (jp *JispProgram) ensureInitialized() {
	if jp.Stack == nil {
		jp.Stack = []interface{}{}
	}
	if jp.Variables == nil {
		jp.Variables = make(map[string]interface{})
	}
	if jp.CallStack == nil {
		jp.CallStack = []*CallFrame{}
	}
}

func (jp *JispProgram) processImports() error {
	if len(jp.Imports) == 0 {
		return nil
	}

	jp.ensureInitialized()

	for _, imp := range jp.Imports {
		if len(imp.Path) > 0 {
			libName := imp.Path[0]
			// For now, only handle local jisp imports.
			// Try .jisp then .json
			filename := libName + ".jisp"
			data, err := os.ReadFile(filename)
			if err != nil {
				filename = libName + ".json"
				data, err = os.ReadFile(filename)
				if err != nil {
					return fmt.Errorf("import error: could not read file for import '%s': %w", libName, err)
				}
			}

			var importedCode interface{}
			if err := json.Unmarshal(data, &importedCode); err != nil {
				return fmt.Errorf("import error: could not parse JSON for import '%s': %w", libName, err)
			}
			jp.Variables[libName] = importedCode
		}
		// TODO: Handle URL imports later.
	}
	return nil
}

// JispOperation represents a single instruction in a JISP program, consisting
// of an operation name and a list of arguments.
type JispOperation struct {
	Name string        `json:"op_name"`   // Not directly from JSON, but will be set by UnmarshalJSON
	Args []interface{} `json:"args_list"` // Not directly from JSON, but will be set by UnmarshalJSON
}

func (op *JispOperation) MarshalJSON() ([]byte, error) {
	raw := make([]interface{}, 0, 1+len(op.Args))
	raw = append(raw, op.Name)
	raw = append(raw, op.Args...)
	return json.Marshal(raw)
}

func (op *JispOperation) UnmarshalJSON(data []byte) error {
	var raw []interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("JispOperation UnmarshalJSON: expected an array, got %w", err)
	}

	parsedOp, err := parseRawOperation(raw)
	if err != nil {
		return fmt.Errorf("JispOperation UnmarshalJSON: %w", err)
	}
	*op = parsedOp // Assign the parsed operation to the receiver
	return nil
}

type operationHandler func(jp *JispProgram, op *JispOperation) error

// makeNoArgsHandler is a higher-order function that wraps a JispProgram method
// to create an operationHandler that enforces no arguments are passed.
func makeNoArgsHandler(method func(*JispProgram) error) operationHandler {
	return func(jp *JispProgram, op *JispOperation) error {
		if len(op.Args) > 0 {
			return fmt.Errorf("%s error: expected 0 arguments, got %d", op.Name, len(op.Args))
		}
		return method(jp)
	}
}

// makeStringUnaryOpHandler creates a handler for simple string unary operations.
func makeStringUnaryOpHandler(op func(string) string) operationHandler {
	return func(jp *JispProgram, opDef *JispOperation) error {
		if len(opDef.Args) > 0 {
			return fmt.Errorf("%s error: expected 0 arguments, got %d", opDef.Name, len(opDef.Args))
		}
		return jp.applyStringUnaryOp(opDef.Name, op)
	}
}

// makeConstantErrorHandler creates a handler that returns a constant error (or nil).
func makeConstantErrorHandler(err error) operationHandler {
	return func(jp *JispProgram, op *JispOperation) error {
		if len(op.Args) > 0 {
			return fmt.Errorf("%s error: expected 0 arguments, got %d", op.Name, len(op.Args))
		}
		return err
	}
}

// makeCollectionOpHandler creates a handler for collection-based operations.
func makeCollectionOpHandler(handlers collectionHandlers) operationHandler {
	return func(jp *JispProgram, op *JispOperation) error {
		return applyCollectionOp(jp, op.Name, op, handlers)
	}
}

var operations map[string]operationHandler

// jispProgramFromBytes reconstructs a JispProgram from raw JSON bytes.
// It's a helper for step/undo to ensure a full JispProgram struct is created from a generic value.
func jispProgramFromBytes(data []byte) (*JispProgram, error) {
	var jp JispProgram
	if err := json.Unmarshal(data, &jp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON into JispProgram struct: %w", err)
	}
	jp.ensureInitialized()
	return &jp, nil
}

// popSubProgram is a helper to pop a value from the stack and correctly parse it into a JispProgram.
func (jp *JispProgram) popSubProgram(opName string) (*JispProgram, error) {
	subProgramVal, err := jp.popValue(opName)
	if err != nil {
		return nil, err
	}

	subProgramBytes, err := json.Marshal(subProgramVal)
	if err != nil {
		return nil, fmt.Errorf("%s error: failed to marshal sub-program value: %w", opName, err)
	}

	subProgram, err := jispProgramFromBytes(subProgramBytes)
	if err != nil {
		return nil, fmt.Errorf("%s error: could not reconstruct sub-program from stack value: %w", opName, err)
	}
	return subProgram, nil
}

func stepOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("step error: expected 0 arguments, got %d", len(op.Args))
	}

	subProgram, err := jp.popSubProgram("step")
	if err != nil {
		return err
	}

	if len(subProgram.Code) == 0 {
		jp.Push(subProgram)
		return nil
	}

	if subProgram.currentFrame() == nil {
		// If the sub-program hasn't started yet, initialize its call stack
		frame := &CallFrame{
			Ops:       subProgram.Code,
			Ip:        0,
			BasePath:  []interface{}{"code"},
			Variables: subProgram.Variables,
		}
		subProgram.CallStack = append(subProgram.CallStack, frame)
	}

	if frame := subProgram.currentFrame(); frame != nil && frame.Ip < len(frame.Ops) && subProgram.SaveHistory {
		before, err := json.Marshal(subProgram)
		if err != nil {
			return fmt.Errorf("step error: failed to snapshot sub-program: %w", err)
		}

		err = subProgram.executeSingleInstruction() // Execute one instruction
		if err != nil && !errors.Is(err, ErrBreakpoint) {
			return fmt.Errorf("step error: during single instruction execution: %w", err)
		}

		after, err := json.Marshal(subProgram)
		if err != nil {
			return fmt.Errorf("step error: failed to marshal post-execution state: %w", err)
		}

		patch, err := CreatePatch(after, before)
		if err != nil {
			return fmt.Errorf("step error: failed to generate diff: %w", err)
		}

		patchBytes, err := patch.MarshalJSON()
		if err != nil {
			return fmt.Errorf("step error: failed to marshal patch: %w", err)
		}
		subProgram.History = append(subProgram.History, patchBytes)
	} else if frame != nil && frame.Ip < len(frame.Ops) {
		err := subProgram.executeSingleInstruction() // Execute one instruction
		if err != nil && !errors.Is(err, ErrBreakpoint) {
			return fmt.Errorf("step error: during single instruction execution: %w", err)
		}
	}
	jp.Push(subProgram)
	return nil
}

func undoOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("undo error: expected 0 arguments, got %d", len(op.Args))
	}

	subProgramVal, err := jp.popValue("undo")
	if err != nil {
		return err
	}

	subProgramBytes, err := json.Marshal(subProgramVal)
	if err != nil {
		return fmt.Errorf("undo error: failed to marshal sub-program value: %w", err)
	}

	subProgram, err := jispProgramFromBytes(subProgramBytes)
	if err != nil {
		return fmt.Errorf("undo error: could not reconstruct sub-program from stack value: %w", err)
	}

	if len(subProgram.History) == 0 {
		return fmt.Errorf("undo error: no history to undo")
	}

	lastPatchBytes := subProgram.History[len(subProgram.History)-1]

	patch, err := DecodePatch(lastPatchBytes)
	if err != nil {
		return fmt.Errorf("undo error: failed to decode patch: %w", err)
	}

	// Apply the patch to revert the state
	revertedBytes, err := patch.Apply(subProgramBytes)
	if err != nil {
		return fmt.Errorf("undo error: failed to apply patch: %w", err)
	}

	// Unmarshal the reverted state back into a JispProgram
	var revertedProgram JispProgram
	if err := json.Unmarshal(revertedBytes, &revertedProgram); err != nil {
		return fmt.Errorf("undo error: failed to unmarshal reverted program: %w", err)
	}

	// Manually unmarshal the CallStack to ensure the custom UnmarshalJSON is applied
	if revertedProgram.CallStack != nil {
		for i, frame := range revertedProgram.CallStack {
			frameBytes, err := json.Marshal(frame)
			if err != nil {
				return fmt.Errorf("undo error: failed to marshal call frame: %w", err)
			}
			var newFrame CallFrame
			if err := json.Unmarshal(frameBytes, &newFrame); err != nil {
				return fmt.Errorf("undo error: failed to unmarshal call frame: %w", err)
			}
			revertedProgram.CallStack[i] = &newFrame
		}
	}

	// Remove the applied patch from the history
	revertedProgram.History = revertedProgram.History[:len(revertedProgram.History)-1]

	jp.Push(&revertedProgram)
	return nil
}

func (jp *JispProgram) Run() error {
	if jp.currentFrame() == nil {
		if len(jp.Code) == 0 {
			return nil
		}
		// Program hasn't started, create initial frame.
		frame := &CallFrame{
			Ops:       jp.Code,
			Ip:        0,
			BasePath:  []interface{}{"code"},
			Variables: jp.Variables,
		}
		jp.CallStack = append(jp.CallStack, frame)
	}

	for {
		frame := jp.currentFrame()
		if frame == nil {
			return nil // Execution finished.
		}

		// Check if current frame is finished
		if frame.Ip >= len(frame.Ops) {
			jp.CallStack = jp.CallStack[:len(jp.CallStack)-1] // Pop frame
			continue
		}

		if jp.Error != nil {
			return nil // Program has an error, stop.
		}

		err := jp.executeSingleInstruction()
		if err != nil {
			if errors.Is(err, ErrBreakpoint) {
				return nil // Breakpoint hit, stop execution gracefully.
			}
			if errors.Is(err, ErrReturn) {
				jp.CallStack = jp.CallStack[:len(jp.CallStack)-1] // Pop frame on return
				continue
			}
			if errors.Is(err, ErrExit) {
				return ErrExit // Exit signal
			}
			// For unhandled control flow like break/continue outside a loop, create an error.
			return nil
		}
	}
}

func breakpointOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) > 0 {
		return fmt.Errorf("breakpoint error: expected 0 arguments, got %d", len(op.Args))
	}
	return ErrBreakpoint
}

func runOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("run error: expected 0 arguments, got %d", len(op.Args))
	}

	subProgram, err := jp.popSubProgram("run")
	if err != nil {
		return err
	}

	err = subProgram.Run()
	// The only error Run is expected to return is ErrExit. Others are stored in subProgram.Error
	if err != nil && !errors.Is(err, ErrExit) {
		return fmt.Errorf("run error: unexpected error during sub-program execution: %w", err)
	}

	jp.Push(subProgram)
	return nil
}

func spawnOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("spawn error: expected 0 arguments, got %d", len(op.Args))
	}

	subProgram, err := jp.popSubProgram("spawn")
	if err != nil {
		return err
	}

	subProgram.done = make(chan struct{})
	subProgram.runningMutex.Lock()
	subProgram.Running = true
	subProgram.runningMutex.Unlock()

	processManager.Register(subProgram)
	jp.Push(subProgram)

	go func() {
		defer func() {
			subProgram.runningMutex.Lock()
			subProgram.Running = false
			subProgram.runningMutex.Unlock()
			close(subProgram.done)
		}()
		err := subProgram.Run()
		if err != nil && !errors.Is(err, ErrExit) {
			log.Printf("spawn error: unexpected error during sub-program execution: %v", err)
		}
	}()

	return nil
}

func toStringOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) > 0 {
		return fmt.Errorf("to_string error: expected 0 arguments, got %d", len(op.Args))
	}
	val, err := jp.popValue("to_string")
	if err != nil {
		return err
	}
	jp.Push(fmt.Sprintf("%v", val))
	return nil
}

func awaitOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("await error: expected 0 arguments, got %d", len(op.Args))
	}

	subProgramVal, err := jp.popValue("await")
	if err != nil {
		return err
	}

	subProgramHandle, ok := subProgramVal.(*JispProgram)
	if !ok {
		// If it's not a direct pointer, it might be a map from JSON unmarshaling.
		// We'll marshal and unmarshal to standardize it.
		subProgramBytes, err := json.Marshal(subProgramVal)
		if err != nil {
			return fmt.Errorf("await error: failed to marshal sub-program handle: %w", err)
		}
		subProgramHandle, err = jispProgramFromBytes(subProgramBytes)
		if err != nil {
			return fmt.Errorf("await error: could not reconstruct sub-program handle from stack value: %w", err)
		}
	}

	if subProgramHandle.PID == "" {
		return fmt.Errorf("await error: program object on stack has no PID, was it spawned?")
	}

	canonicalProgram, ok := processManager.Get(subProgramHandle.PID)
	if !ok {
		// This could mean the program finished and was reaped, or the PID is invalid.
		// For now, we'll assume it's just not found.
		return fmt.Errorf("await error: no running program found for PID '%s'", subProgramHandle.PID)
	}

	if canonicalProgram.done != nil {
		<-canonicalProgram.done
	}

	jp.Push(canonicalProgram)
	return nil
}

func concatOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) > 0 {
		return fmt.Errorf("concat error: expected 0 arguments, got %d", len(op.Args))
	}
	v1, v2, err := popTwo[string](jp, "concat")
	if err != nil {
		return err
	}
	jp.Push(v1 + v2)
	return nil
}

func lenStringHandler(s string) (interface{}, error)                 { return float64(len(s)), nil }
func lenArrayHandler(a []interface{}) (interface{}, error)           { return float64(len(a)), nil }
func lenObjectHandler(m map[string]interface{}) (interface{}, error) { return float64(len(m)), nil }

func keysObjectHandler(m map[string]interface{}) (interface{}, error) {
	keys := make([]interface{}, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys, nil
}

func valuesObjectHandler(m map[string]interface{}) (interface{}, error) {
	values := make([]interface{}, 0, len(m))
	for _, val := range m {
		values = append(values, val)
	}
	return values, nil
}

func replaceOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) > 0 {
		return fmt.Errorf("replace error: expected 0 arguments, got %d", len(op.Args))
	}
	str, old, new, err := popThree[string, string, string](jp, "replace")
	if err != nil {
		return err
	}
	jp.Push(strings.ReplaceAll(str, old, new))
	return nil
}

func sliceOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) > 0 {
		return fmt.Errorf("slice error: expected 0 arguments, got %d", len(op.Args))
	}
	var inputVal, startRaw, endRaw interface{}

	// Try to pop 3 values (input, start, end)
	values, err := jp.popx("slice", 3)
	if err == nil {
		inputVal, startRaw, endRaw = values[0], values[1], values[2]
	} else {
		// If 3 values not available, try to pop 2 values (input, start)
		values, err = jp.popx("slice", 2)
		if err != nil {
			return fmt.Errorf("slice error: stack underflow, expected at least 2 values (input, start)")
		}
		inputVal, startRaw = values[0], values[1]
		endRaw = nil // Explicitly set endRaw to nil if not provided
	}

	startFloat, ok := startRaw.(float64)
	if !ok {
		return fmt.Errorf("slice error: expected numeric start index, got %T", startRaw)
	}
	start := int(startFloat)

	hasEnd := endRaw != nil
	var end int
	if hasEnd {
		endFloat, ok := endRaw.(float64)
		if !ok {
			return fmt.Errorf("slice error: expected numeric end index, got %T", endRaw)
		}
		end = int(endFloat)
	}

	var length int
	switch v := inputVal.(type) {
	case string:
		length = len(v)
	case []interface{}:
		length = len(v)
	default:
		return fmt.Errorf("slice error: unsupported type %T for slicing, expected string or array", inputVal)
	}

	if !hasEnd {
		end = length
	}

	if start < 0 || end < start || end > length {
		return fmt.Errorf("slice error: invalid indices [%d:%d] for collection of length %d", start, end, length)
	}

	switch v := inputVal.(type) {
	case string:
		jp.Push(v[start:end])
	case []interface{}:
		jp.Push(v[start:end])
	}

	return nil
}

func init() {
	operations = map[string]operationHandler{
		"push":         pushOp,
		"pop":          popOp,
		"set":          setOp,
		"get":          getOp,
		"if":           ifOp,
		"while":        whileOp,
		"raise":        raiseOp,
		"assert":       assertOp,
		"await":        awaitOp,
		"range":        rangeOp,
		"foreach":      forOp,
		"filter":       filterOp,
		"map":          mapOp,
		"reduce":       reduceOp,
		"sort":         sortOp,
		"union":        unionOp,
		"intersection": intersectionOp,
		"difference":   differenceOp,
		"join":         joinOp,
		"valid":        validOp,
		"call":         callOp,
		"return":       returnOp,
		"exit":         exitOp,
		"step":         stepOp,
		"undo":         undoOp,
		"run":          runOp,
		"spawn":        spawnOp,
		"breakpoint":   breakpointOp,
		"to_string":    toStringOp,
		"concat":       concatOp,
		"try":          tryOp,
		"replace":      replaceOp,
		"for":          forOp,
		"slice":        sliceOp,
		"exists":       makeNoArgsHandler((*JispProgram).Exists),
		"delete":       makeNoArgsHandler((*JispProgram).Delete),
		"eq":           makeNoArgsHandler((*JispProgram).Eq),
		"lt":           makeNoArgsHandler((*JispProgram).Lt),
		"gt":           makeNoArgsHandler((*JispProgram).Gt),
		"add":          makeNoArgsHandler((*JispProgram).Add),
		"sub":          makeNoArgsHandler((*JispProgram).Sub),
		"mul":          makeNoArgsHandler((*JispProgram).Mul),
		"div":          makeNoArgsHandler((*JispProgram).Div),
		"mod":          makeNoArgsHandler((*JispProgram).Mod),
		"and":          makeNoArgsHandler((*JispProgram).And),
		"or":           makeNoArgsHandler((*JispProgram).Or),
		"not":          makeNoArgsHandler((*JispProgram).Not),
		"trim":         makeStringUnaryOpHandler(strings.TrimSpace),
		"lower":        makeStringUnaryOpHandler(strings.ToLower),
		"upper":        makeStringUnaryOpHandler(strings.ToUpper),
		"break":        makeConstantErrorHandler(ErrBreak),
		"continue":     makeConstantErrorHandler(ErrContinue),
		"noop":         makeConstantErrorHandler(nil),
		"len": makeCollectionOpHandler(collectionHandlers{
			stringHandler: lenStringHandler,
			arrayHandler:  lenArrayHandler,
			objectHandler: lenObjectHandler,
		}),
		"keys": makeCollectionOpHandler(collectionHandlers{
			objectHandler: keysObjectHandler,
		}),
		"values": makeCollectionOpHandler(collectionHandlers{
			objectHandler: valuesObjectHandler,
		}),
	}
}

func validOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("valid error: expected 0 arguments, got %d", len(op.Args))
	}

	values, err := jp.popx("valid", 2)
	if err != nil {
		return fmt.Errorf("valid error: %w", err)
	}
	schemaValue := values[0]
	docValue := values[1]

	// Marshal the schemaValue to JSON string for compilation
	schemaBytes, err := json.Marshal(schemaValue)
	if err != nil {
		return fmt.Errorf("valid error: failed to marshal schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", strings.NewReader(string(schemaBytes))); err != nil {
		return fmt.Errorf("valid error: failed to add schema resource: %w", err)
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("valid error: failed to compile schema: %w", err)
	}

	err = schema.Validate(docValue)
	jp.Push(err == nil)

	return nil
}

func toComparableSlice(input []interface{}, opName string) ([]interface{}, error) {
	for _, item := range input {
		switch item.(type) {
		case float64, string, bool:
			// These are comparable types
		case nil:
			// nil is also comparable
		default:
			return nil, fmt.Errorf("%s error: unsupported type %T in array, expected number, string, boolean or null", opName, item)
		}
	}
	return input, nil
}

func unique(slice []interface{}) []interface{} {
	allKeys := make(map[interface{}]bool)
	list := []interface{}{}
	for _, entry := range slice {
		if _, value := allKeys[entry]; !value {
			allKeys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func isSliceOfType[T any](slice []interface{}) bool {
	for _, item := range slice {
		if _, ok := item.(T); !ok {
			return false
		}
	}
	return true
}

func popTwoComparableSlices(jp *JispProgram, opName string) ([]interface{}, []interface{}, error) {
	a1, a2, err := popTwo[[]interface{}](jp, opName)
	if err != nil {
		return nil, nil, err
	}

	if _, err := toComparableSlice(a1, opName); err != nil {
		return nil, nil, err
	}
	if _, err := toComparableSlice(a2, opName); err != nil {
		return nil, nil, err
	}

	return a1, a2, nil
}

func unionOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("union error: expected 0 arguments, got %d", len(op.Args))
	}

	a1, a2, err := popTwoComparableSlices(jp, "union")
	if err != nil {
		return err
	}

	combined := append(a1, a2...)
	jp.Push(unique(combined))
	return nil
}

func intersectionOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("intersection error: expected 0 arguments, got %d", len(op.Args))
	}

	a1, a2, err := popTwoComparableSlices(jp, "intersection")
	if err != nil {
		return err
	}

	hashSet := make(map[interface{}]bool)
	for _, x := range a1 {
		hashSet[x] = true
	}

	var result []interface{}
	for _, x := range a2 {
		if hashSet[x] {
			result = append(result, x)
			delete(hashSet, x) // Ensure unique elements in intersection
		}
	}
	jp.Push(result)
	return nil
}

func differenceOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("difference error: expected 0 arguments, got %d", len(op.Args))
	}

	a1, a2, err := popTwoComparableSlices(jp, "difference")
	if err != nil {
		return err
	}

	hashSet := make(map[interface{}]bool)
	for _, x := range a2 {
		hashSet[x] = true
	}

	var result []interface{}
	for _, x := range a1 {
		if !hashSet[x] {
			result = append(result, x)
		}
	}
	jp.Push(unique(result))
	return nil
}

func joinOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("join error: expected 0 arguments, got %d", len(op.Args))
	}

	args, err := jp.popx("join", 5)
	if err != nil {
		return err
	}

	leftArray, ok := args[0].([]interface{})
	if !ok {
		return fmt.Errorf("join error: expected an array on stack for left array, got %T", args[0])
	}
	rightArray, ok := args[1].([]interface{})
	if !ok {
		return fmt.Errorf("join error: expected an array on stack for right array, got %T", args[1])
	}

	leftName, ok := args[2].(string)
	if !ok {
		return fmt.Errorf("join error: expected a string on stack for left name, got %T", args[2])
	}
	rightName, ok := args[3].(string)
	if !ok {
		return fmt.Errorf("join error: expected a string on stack for right name, got %T", args[3])
	}

	joinOps, err := parseJispOps(args[4])
	if err != nil {
		return fmt.Errorf("join error: invalid operations block: %w", err)
	}

	var result []interface{}

	for _, leftItem := range leftArray {
		for _, rightItem := range rightArray {
			jp.Variables[leftName] = leftItem
			jp.Variables[rightName] = rightItem

			if err := jp.executeOperationsWithPathSegment(joinOps, "join_ops_from_stack", false); err != nil {
				return err
			}

			condition, err := pop[bool](jp, "join")
			if err != nil {
				return err
			}

			if condition {
				result = append(result, map[string]interface{}{
					leftName:  leftItem,
					rightName: rightItem,
				})
			}
		}
	}

	jp.Push(result)
	return nil
}

func sortOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("sort error: expected 0 arguments, got %d", len(op.Args))
	}

	val, err := jp.popValue("sort")
	if err != nil {
		return fmt.Errorf("sort error: %w", err)
	}

	switch v := val.(type) {
	case []interface{}:
		if len(v) == 0 {
			jp.Push(v)
			return nil
		}

		// Check type of first element to determine sort type
		switch v[0].(type) {
		case float64:
			if !isSliceOfType[float64](v) {
				return fmt.Errorf("sort error: array contains mixed types")
			}
			sort.Slice(v, func(i, j int) bool {
				return v[i].(float64) < v[j].(float64)
			})
			jp.Push(v)
			return nil
		case string:
			if !isSliceOfType[string](v) {
				return fmt.Errorf("sort error: array contains mixed types")
			}
			sort.Slice(v, func(i, j int) bool {
				return v[i].(string) < v[j].(string)
			})
			jp.Push(v)
			return nil
		default:
			return fmt.Errorf("sort error: array contains unsortable types")
		}
	default:
		return fmt.Errorf("sort error: unsupported type %T for sorting, expected array", val)
	}
}

func reduceOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("reduce error: expected 0 arguments, got %d", len(op.Args))
	}

	// For reduce, we pop 3 arguments: input array, body operations, and initial value.
	// The helper popCollectionOpArgs pops input, varName, ops. We need to adjust.
	args, err := jp.popx("reduce", 3)
	if err != nil {
		return err
	}

	input, ok := args[0].([]interface{})
	if !ok {
		return fmt.Errorf("reduce error: expected an array on stack for input, got %T", args[0])
	}

	reduceOps, err := parseJispOps(args[1])
	if err != nil {
		return fmt.Errorf("reduce error: invalid operations block: %w", err)
	}

	initialValue := args[2]

	accumulator := initialValue

	for _, item := range input {
		jp.Push(accumulator)
		jp.Push(item)

		previousStackLen := len(jp.Stack)

		if err := jp.executeOperationsWithPathSegment(reduceOps, "reduce_ops_from_stack", false); err != nil {
			return err
		}

		if len(jp.Stack) == previousStackLen {
			return fmt.Errorf("reduce error: operations block did not push a result to the stack")
		}
		accumulator, err = jp.popValue("reduce")
		if err != nil {
			return err
		}
	}

	jp.Push(accumulator)
	return nil
}

func mapOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("map error: expected 0 arguments, got %d", len(op.Args))
	}

	input, varName, mapOps, err := jp.popCollectionOpArgs("map", 3)
	if err != nil {
		return err
	}

	result, err := applyCollectionLoop(jp, "map", input, varName, mapOps, "map_ops_from_stack",
		func(jp *JispProgram, item interface{}, varName string, bodyOps []JispOperation, pathSegment string) (interface{}, error) {
			jp.Variables[varName] = item
			if err := jp.executeOperationsWithPathSegment(bodyOps, pathSegment, false); err != nil {
				return nil, err
			}
			res, err := jp.popValue("map")
			if err != nil {
				return nil, err
			}
			return res, nil
		})
	if err != nil {
		return err
	}

	jp.Push(result)
	return nil
}

func filterOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("filter error: expected 0 arguments, got %d", len(op.Args))
	}

	input, varName, conditionOps, err := jp.popCollectionOpArgs("filter", 3)
	if err != nil {
		return err
	}

	result, err := applyCollectionLoop(jp, "filter", input, varName, conditionOps, "filter_ops_from_stack",
		func(jp *JispProgram, item interface{}, varName string, bodyOps []JispOperation, pathSegment string) (interface{}, error) {
			jp.Variables[varName] = item
			if err := jp.executeOperationsWithPathSegment(bodyOps, pathSegment, false); err != nil {
				return nil, err
			}
			condition, err := pop[bool](jp, "filter")
			if err != nil {
				return nil, err
			}
			if condition {
				return item, nil
			}
			return nil, nil
		})
	if err != nil {
		return err
	}

	var filteredResult []interface{}
	for _, item := range result {
		if item != nil {
			filteredResult = append(filteredResult, item)
		}
	}

	jp.Push(filteredResult)
	return nil
}

func rangeOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("range error: expected 0 arguments, got %d", len(op.Args))
	}

	start, stop, step, err := popThree[float64, float64, float64](jp, "range")
	if err != nil {
		return err
	}

	var result []interface{}
	for i := start; i < stop; i += step {
		result = append(result, i)
	}
	jp.Push(result)
	return nil
}

func raiseOp(jp *JispProgram, op *JispOperation) error {
	errmsg, err := pop[string](jp, "raise")
	if err != nil {
		return err
	}
	jp.Error = jp.newError(op, errmsg)
	return nil
}

func assertOp(jp *JispProgram, op *JispOperation) error {
	val, err := jp.popValue("assert")
	if err != nil {
		return err
	}

	condition, ok := val.(bool)
	if !ok {
		return fmt.Errorf("assert error: expected a boolean on the stack, got %T", val)
	}

	if !condition {
		errmsg := "assertion failed"
		if len(op.Args) > 0 {
			if customMsg, ok := op.Args[0].(string); ok {
				errmsg = customMsg
			}
		}
		jp.Error = jp.newError(op, errmsg)
	}

	return nil
}

func (jp *JispProgram) currentInstructionPath() []interface{} {
	frame := jp.currentFrame()
	if frame == nil {
		return nil
	}
	// The full path is the base path of the current operation list plus the instruction pointer.
	return append(frame.BasePath, frame.Ip)
}

func (jp *JispProgram) executeOperationsWithPathSegment(ops []JispOperation, segment interface{}, useParentScope bool) error {
	parentPath := jp.currentInstructionPath()
	// It's crucial to copy the parentPath to avoid mutations across different branches of execution.
	path := make([]interface{}, len(parentPath)+1)
	copy(path, parentPath)
	path[len(parentPath)] = segment
	return jp.ExecuteFrame(ops, path, useParentScope, -1)
}

func (jp *JispProgram) executeSingleInstruction() error {
	frame := jp.currentFrame()
	op := frame.Ops[frame.Ip]

	handler, found := operations[op.Name]
	if !found {
		jp.Error = jp.newError(&op, fmt.Sprintf("unknown operation: %s", op.Name))
		frame.Ip++ // Consume the invalid instruction and stop for now.
		return nil
	}

	// Execute the operation first.
	err := handler(jp, &op)

	// Now, handle errors and IP increment.
	// We always advance the IP, even on JispError, to prevent infinite loops.
	frame.Ip++

	if err != nil {
		var jispErr *JispError
		switch {
		// Control flow signals are propagated up to be handled by the execution loops (Run, For, etc.)
		case errors.Is(err, ErrBreak), errors.Is(err, ErrContinue), errors.Is(err, ErrReturn), errors.Is(err, ErrExit):
			return err
		// A breakpoint from a `breakpoint` instruction is also a control flow signal.
		case errors.Is(err, ErrBreakpoint):
			return err
		// JispErrors are caught by 'try' or halt execution. We set them on the program state.
		case errors.As(err, &jispErr):
			jp.Error = jispErr
		// Any other error is a runtime error that becomes a JispError.
		default:
			jp.Error = jp.newError(&op, err.Error())
		}
		// After setting the error, we stop further execution in the current frame.
		return nil
	}

	// AFTER successful execution and IP increment, check if the NEXT instruction is a breakpoint.
	if jp.Debug && len(jp.Breakpoints) > 0 {
		// Check if we're still inside the code boundary.
		if frame.Ip < len(frame.Ops) {
			currentPath := jp.currentInstructionPath()
			for _, bp := range jp.Breakpoints {
				if pathsEqual(currentPath, bp) {
					// We've landed on a breakpoint, so we signal to the caller to stop.
					return ErrBreakpoint
				}
			}
		}
	}

	// No errors, no breakpoint, continue execution.
	return nil
}

// ExecuteOperations pushes a new call frame for the given operations and executes them.
// It manages the instruction pointer within this frame and handles control flow.
func (jp *JispProgram) ExecuteFrame(ops []JispOperation, BasePath []interface{}, useParentScope bool, instructionLimit int) error {
	if len(ops) == 0 {
		return nil
	}

	var frameVars map[string]interface{}
	parentFrame := jp.currentFrame() // Get parent *before* pushing new one

	if useParentScope && parentFrame != nil {
		frameVars = parentFrame.Variables
	} else if parentFrame == nil {
		frameVars = jp.Variables // Global scope
	} else {
		frameVars = make(map[string]interface{}) // New scope
	}

	frame := &CallFrame{
		Ops:       ops,
		Ip:        0,
		BasePath:  BasePath,
		Variables: frameVars,
	}
	jp.CallStack = append(jp.CallStack, frame)

	defer func() {
		if len(jp.CallStack) > 0 && jp.CallStack[len(jp.CallStack)-1] == frame {
			jp.CallStack = jp.CallStack[:len(jp.CallStack)-1]
		}
	}()

	for instructionLimit != 0 && frame.Ip < len(frame.Ops) {
		if jp.Error != nil {
			return nil // Stop if a runtime error was set
		}

		err := jp.executeSingleInstruction()
		if err != nil {
			if errors.Is(err, ErrBreakpoint) {
				return nil // Stop execution for this frame on breakpoint.
			}
			if errors.Is(err, ErrReturn) {
				return nil // This frame is done.
			}
			return err // Propagate break, continue, exit
		}
		instructionLimit--
	}
	return nil
}

func toFloat(v interface{}) (float64, bool) {
	switch i := v.(type) {
	case float64:
		return i, true
	case int:
		return float64(i), true
	case int32:
		return float64(i), true
	case int64:
		return float64(i), true
	default:
		return 0, false
	}
}

func pathsEqual(p1, p2 []interface{}) bool {
	if len(p1) != len(p2) {
		return false
	}
	for i := range p1 {
		v1 := p1[i]
		v2 := p2[i]

		f1, ok1 := toFloat(v1)
		f2, ok2 := toFloat(v2)

		if ok1 && ok2 {
			if f1 != f2 {
				return false
			}
		} else if !reflect.DeepEqual(v1, v2) {
			return false
		}
	}
	return true
}

func callOp(jp *JispProgram, op *JispOperation) error {
	// Pop the function to be called from the stack.
	funcVal, err := jp.popValue("call")
	if err != nil {
		return err
	}

	var funcOps []JispOperation

	switch fn := funcVal.(type) {
	case string:
		// If it's a string, get the function code from variables.
		// This will fetch from the current scope first, then global.
		code, err := jp.getValueForPath(fn)
		if err != nil {
			return fmt.Errorf("call error: could not find function '%s': %w", fn, err)
		}
		funcOps, err = parseJispOps(code)
		if err != nil {
			return fmt.Errorf("call error: invalid operations block for function '%s': %w", fn, err)
		}
	case []interface{}:
		// If it's raw code, parse it.
		var err error
		funcOps, err = parseJispOps(fn)
		if err != nil {
			return fmt.Errorf("call error: invalid raw operations block: %w", err)
		}
	default:
		return fmt.Errorf("call error: expected a function name (string) or raw function code (array) on the stack, got %T", funcVal)
	}

	// Execute the function's operations.
	err = jp.executeOperationsWithPathSegment(funcOps, "function_call", false)
	if err != nil && !errors.Is(err, ErrReturn) {
		return err // It was a real error, not a return.
	}
	return nil // It was a normal return, so we continue.
}

func returnOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) > 0 {
		return fmt.Errorf("return error: expected 0 arguments, got %d", len(op.Args))
	}
	// A call stack length of 1 means it's the global/main execution frame.
	// Returning from the global scope is an error.
	if len(jp.CallStack) == 1 {
		return fmt.Errorf("return error: return can only be called within a function execution context")
	}
	return ErrReturn
}

func pushOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) == 0 {
		return fmt.Errorf("push error: no argument provided")
	}
	jp.Push(op.Args[0])
	return nil
}

func popOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) == 0 {
		return fmt.Errorf("pop error: no argument provided for field name")
	}
	fieldName, ok := op.Args[0].(string)
	if !ok {
		return fmt.Errorf("pop error: expected string argument for fieldName, got %T", op.Args[0])
	}
	return jp.Pop(fieldName)
}

func ifOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) == 0 || len(op.Args) > 2 {
		return fmt.Errorf("if error: expected 1 or 2 array arguments for then/else bodies, got %v", op.Args)
	}

	thenBody, err := parseJispOps(op.Args[0])
	if err != nil {
		return fmt.Errorf("if error in 'then' body: %w", err)
	}

	var elseBody []JispOperation
	if len(op.Args) == 2 {
		elseBody, err = parseJispOps(op.Args[1])
		if err != nil {
			return fmt.Errorf("if error in 'else' body: %w", err)
		}
	}
	return jp.If(thenBody, elseBody)
}

func tryOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) < 2 || len(op.Args) > 3 {
		return fmt.Errorf("try error: expected 2 or 3 arguments for try_body, catch_var, and optional catch_body, got %v", op.Args)
	}

	tryBody, err := parseJispOps(op.Args[0])
	if err != nil {
		return fmt.Errorf("try error in 'try_body': %w", err)
	}

	catchVar, ok := op.Args[1].(string)
	if !ok {
		return fmt.Errorf("try error: expected catch_var to be a string, got %T", op.Args[1])
	}

	var catchBody []JispOperation
	if len(op.Args) == 3 {
		catchBody, err = parseJispOps(op.Args[2])
		if err != nil {
			return fmt.Errorf("try error in 'catch_body': %w", err)
		}
	}

	return jp.Try(tryBody, catchVar, catchBody)
}

func forOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 3 {
		return fmt.Errorf("for error: expected 3 arguments: loop_var, collection, body_operations, got %v", op.Args)
	}

	loopVar, ok := op.Args[0].(string)
	if !ok {
		return fmt.Errorf("for error: expected loop_var to be a string, got %T", op.Args[0])
	}

	collectionVal := op.Args[1]

	if collectionStr, ok := collectionVal.(string); ok {
		// If collection is a string, treat it as a variable name and get its value
		resolvedCollection, err := jp.getValueForPath(collectionStr)
		if err != nil {
			return fmt.Errorf("for error: failed to get collection variable '%s': %w", collectionStr, err)
		}
		collectionVal = resolvedCollection
	}

	bodyOps, err := parseJispOps(op.Args[2])
	if err != nil {
		return fmt.Errorf("for error in 'body_operations': %w", err)
	}

	return jp.For(loopVar, collectionVal, bodyOps, 2)
}

func whileOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 2 {
		return fmt.Errorf("while error: expected 2 arguments for condition path and body, got %v", op.Args)
	}

	conditionPathRaw := op.Args[0]
	conditionPath, ok := conditionPathRaw.(string)
	if !ok {
		return fmt.Errorf("while error: expected condition path to be a string, got %T", conditionPathRaw)
	}

	bodyOps, err := parseJispOps(op.Args[1])
	if err != nil {
		return fmt.Errorf("while error in 'body' operations: %w", err)
	}

	for {
		// Get the value of the condition variable
		conditionVal, err := jp.getValueForPath(conditionPath)
		if err != nil {
			return fmt.Errorf("while error: failed to get condition variable '%s': %w", conditionPath, err)
		}

		condition, ok := conditionVal.(bool)
		if !ok {
			return fmt.Errorf("while error: expected boolean condition at '%s', got %T", conditionPath, conditionVal)
		}

		if !condition {
			break
		}

		if err := jp.executeOperationsWithPathSegment(bodyOps, 1, true); err != nil {
			// Handle break and continue signals
			if errors.Is(err, ErrBreak) {
				break // Exit the while loop
			}
			if errors.Is(err, ErrContinue) {
				continue // Skip to the next iteration of the while loop
			}
			return fmt.Errorf("while error during body execution: %w", err)
		}
	}
	return nil
}

type collectionHandlers struct {
	stringHandler func(string) (interface{}, error)
	arrayHandler  func([]interface{}) (interface{}, error)
	objectHandler func(map[string]interface{}) (interface{}, error)
}

func applyCollectionOp(jp *JispProgram, opName string, op *JispOperation, handlers collectionHandlers) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("%s error: expected 0 arguments, got %d", opName, len(op.Args))
	}

	val, err := jp.popValue(opName)
	if err != nil {
		return fmt.Errorf("%s error: %w", opName, err)
	}

	var result interface{}
	switch v := val.(type) {
	case string:
		if handlers.stringHandler == nil {
			return fmt.Errorf("%s error: unsupported type string", opName)
		}
		result, err = handlers.stringHandler(v)
	case []interface{}:
		if handlers.arrayHandler == nil {
			return fmt.Errorf("%s error: unsupported type array", opName)
		}
		result, err = handlers.arrayHandler(v)
	case map[string]interface{}:
		if handlers.objectHandler == nil {
			return fmt.Errorf("%s error: unsupported type object", opName)
		}
		result, err = handlers.objectHandler(v)
	default:
		return fmt.Errorf("%s error: unsupported type %T", opName, val)
	}

	if err != nil {
		return err
	}

	jp.Push(result)
	return nil
}

// applyCollectionLoop is a helper for map and filter operations.
// It iterates over an input array, sets a loop variable, executes body operations,
// and collects results based on a provided handler.
func applyCollectionLoop(
	jp *JispProgram,
	opName string,
	input []interface{},
	varName string,
	bodyOps []JispOperation,
	pathSegment string,
	handler func(jp *JispProgram, item interface{}, varName string, bodyOps []JispOperation, pathSegment string) (interface{}, error),
) ([]interface{}, error) {
	var result []interface{}
	for _, item := range input {
		res, err := handler(jp, item, varName, bodyOps, pathSegment)
		if err != nil {
			return nil, err
		}
		result = append(result, res)
	}
	return result, nil
}

func (jp *JispProgram) Push(value interface{}) {
	jp.Stack = append(jp.Stack, value)
}

func (jp *JispProgram) Pop(fieldName string) error {
	value, err := jp.popValue("pop")
	if err != nil {
		return err
	}
	jp.ensureInitialized()
	jp.Variables[fieldName] = value
	return nil
}

// navigateToParent traverses a path up to the second-to-last element, returning the container
// of what the last element of the path refers to. It handles auto-vivification for maps.
func (jp *JispProgram) navigateToParent(path []interface{}, autoVivify bool, opName string) (interface{}, error) {
	// TODO: Implement lexical scoping for the root of the path.
	// The first segment of the path should be resolved using the new scope-aware logic.
	// Subsequent segments navigate within the retrieved object as before.
	var current interface{} = jp.Variables
	for i := 0; i < len(path)-1; i++ {
		segment := path[i]
		switch key := segment.(type) {
		case string:
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("%s error: trying to access non-map with string key '%s' in path %v", opName, key, path)
			}
			if next, found := m[key]; found {
				current = next
			} else if autoVivify {
				newMap := make(map[string]interface{})
				m[key] = newMap
				current = newMap
			} else {
				if i == 0 {
					return nil, fmt.Errorf("%s error: variable '%s' not found", opName, key)
				}
				return nil, fmt.Errorf("%s error: key '%s' not found in path %v", opName, key, path)
			}
		case float64:
			index := int(key)
			a, ok := current.([]interface{})
			if !ok {
				return nil, fmt.Errorf("%s error: trying to access non-array with numeric index %d in path %v", opName, index, path)
			}
			if index >= 0 && index < len(a) {
				current = a[index]
			} else {
				return nil, fmt.Errorf("%s error: index %d out of bounds for path %v", opName, index, path)
			}
		default:
			return nil, fmt.Errorf("%s error: invalid path segment type %T in path %v", opName, segment, path)
		}
	}
	return current, nil
}

// setValueForPath stores a value at a given path, which can be a simple string
// for a variable or a slice representing a nested path.
func (jp *JispProgram) setValueForPath(pathVal interface{}, value interface{}) error {
	// TODO: Implement lexical scoping.
	// 1. For a simple string path, set the variable in the current frame's locals.
	// 2. For a complex path `["var", "key"]`, use the scoped `getValueForPath` to find "var"
	//    and then modify it in place.
	jp.ensureInitialized()

	switch path := pathVal.(type) {
	case string:
		frame := jp.currentFrame()
		if frame != nil {
			frame.Variables[path] = value
		} else {
			// If no frame (global scope), set in global variables
			jp.Variables[path] = value
		}
		return nil
	case []interface{}:
		if len(path) == 0 {
			return fmt.Errorf("set error: path array cannot be empty")
		}
		if _, ok := path[0].(string); !ok {
			return fmt.Errorf("set error: first element of path must be a string variable name, got %T", path[0])
		}

		parent, err := jp.navigateToParent(path, true, "set")
		if err != nil {
			return err
		}

		// Set the value at the final path segment
		lastSegment := path[len(path)-1]
		switch key := lastSegment.(type) {
		case string:
			if m, ok := parent.(map[string]interface{}); ok {
				m[key] = value
			} else {
				return fmt.Errorf("set error: final segment of path is a string key '%s' but the target is not a map in path %v", key, path)
			}
		case float64:
			index := int(key)
			if a, ok := parent.([]interface{}); ok {
				if index >= 0 && index < len(a) {
					a[index] = value
				} else {
					return fmt.Errorf("set error: final index %d is out of bounds for path %v", index, path)
				}
			} else {
				return fmt.Errorf("set error: final segment of path is a numeric index %d but the target is not an array in path %v", index, path)
			}
		default:
			return fmt.Errorf("set error: invalid final path segment type %T in path %v", lastSegment, path)
		}
		return nil
	default:
		return fmt.Errorf("set error: expected a string or an array path, got %T", pathVal)
	}
}

func setOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) == 0 {
		// No args: pop value, then pop path from the stack.
		values, err := jp.popx("set", 2)
		if err != nil {
			return err
		}
		// The stack order is [..., value, path], so path is at the top.
		path := values[1]
		val := values[0]
		return jp.setValueForPath(path, val)
	}

	if len(op.Args) == 1 {
		// One arg: pop value from stack, use arg as path.
		pathVal := op.Args[0]
		value, err := jp.popValue("set")
		if err != nil {
			return err
		}
		return jp.setValueForPath(pathVal, value)
	}

	// Multi-args: pop a value for each path argument and set them.
	// The number of values must match the number of paths.
	numArgs := len(op.Args)
	values, err := jp.popx("set", numArgs)
	if err != nil {
		return err
	}

	// Assign values to paths in the correct LIFO order.
	// The last path gets the last popped value (which was at the top of the stack).
	for i := 0; i < numArgs; i++ {
		pathVal := op.Args[i]
		value := values[i]
		if err := jp.setValueForPath(pathVal, value); err != nil {
			return err
		}
	}
	return nil
}

func (jp *JispProgram) getValueByPath(path []interface{}) (interface{}, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("get error: path array cannot be empty")
	}

	if _, ok := path[0].(string); !ok {
		return nil, fmt.Errorf("get error: first element of path must be a string variable name, got %T", path[0])
	}

	parent, err := jp.navigateToParent(path, false, "get")
	if err != nil {
		return nil, err
	}

	lastSegment := path[len(path)-1]
	switch key := lastSegment.(type) {
	case string:
		m, ok := parent.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("get error: trying to access non-map with string key '%s' in path %v", key, path)
		}
		if val, found := m[key]; found {
			return val, nil
		}
		if len(path) == 1 {
			return nil, fmt.Errorf("get error: variable '%s' not found", key)
		}
		return nil, fmt.Errorf("get error: key '%s' not found in path %v", key, path)
	case float64:
		index := int(key)
		a, ok := parent.([]interface{})
		if !ok {
			return nil, fmt.Errorf("get error: trying to access non-array with numeric index %d in path %v", index, path)
		}
		if index >= 0 && index < len(a) {
			return a[index], nil
		}
		return nil, fmt.Errorf("get error: index %d out of bounds for path %v", index, path)
	default:
		return nil, fmt.Errorf("get error: invalid final path segment type %T in path %v", lastSegment, path)
	}
}

func (jp *JispProgram) getValueForPath(pathVal interface{}) (interface{}, error) {
	switch path := pathVal.(type) {
	case string:
		// 1. Search up the call stack.
		for i := len(jp.CallStack) - 1; i >= 0; i-- {
			frame := jp.CallStack[i]
			if val, found := frame.Variables[path]; found {
				return val, nil
			}
		}
		// 2. If not found in any call frame, check global variables.
		val, found := jp.Variables[path]
		if !found {
			return nil, fmt.Errorf("get error: variable '%s' not found", path)
		}
		return val, nil
	case []interface{}:
		return jp.getValueByPath(path)
	default:
		return nil, fmt.Errorf("get error: expected a string or an array path, got %T", pathVal)
	}
}

func getOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) == 0 {
		// No args: use the string or array at the top of the stack as a path.
		pathVal, err := jp.popValue("get")
		if err != nil {
			return err
		}

		val, err := jp.getValueForPath(pathVal)
		if err != nil {
			return err
		}
		jp.Push(val)
		return nil
	}

	// one or multi args: get each of the provided paths in order.
	for _, pathVal := range op.Args {
		val, err := jp.getValueForPath(pathVal)
		if err != nil {
			return err
		}
		jp.Push(val)
	}
	return nil
}

func (jp *JispProgram) Exists() error {
	key, err := pop[string](jp, "exists")
	if err != nil {
		return err
	}
	_, found := jp.Variables[key]
	jp.Push(found)
	return nil
}

func (jp *JispProgram) Delete() error {
	key, err := pop[string](jp, "delete")
	if err != nil {
		return err
	}
	if jp.Variables != nil {
		delete(jp.Variables, key)
	}
	return nil
}

func (jp *JispProgram) Eq() error {
	vals, err := jp.popx("eq", 2)
	if err != nil {
		return err
	}
	jp.Push(vals[0] == vals[1])
	return nil
}

func (jp *JispProgram) Lt() error {
	return jp.applyComparisonOp("lt",
		func(a, b float64) bool { return a < b },
		func(a, b string) bool { return a < b },
	)
}

func (jp *JispProgram) Gt() error {
	return jp.applyComparisonOp("gt",
		func(a, b float64) bool { return a > b },
		func(a, b string) bool { return a > b },
	)
}

func (jp *JispProgram) Add() error {
	return applyBinaryOp[float64](jp, "add", func(a, b float64) (interface{}, error) {
		return a + b, nil
	})
}

func (jp *JispProgram) Sub() error {
	return applyBinaryOp[float64](jp, "sub", func(a, b float64) (interface{}, error) {
		return a - b, nil
	})
}

func (jp *JispProgram) Mul() error {
	return applyBinaryOp[float64](jp, "mul", func(a, b float64) (interface{}, error) {
		return a * b, nil
	})
}

func (jp *JispProgram) Div() error {
	return applyBinaryOp[float64](jp, "div", func(a, b float64) (interface{}, error) {
		if b == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return a / b, nil
	})
}

func (jp *JispProgram) Mod() error {
	return applyBinaryOp[float64](jp, "mod", func(a, b float64) (interface{}, error) {
		if b == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return math.Mod(a, b), nil
	})
}

func (jp *JispProgram) And() error {
	return applyBinaryOp[bool](jp, "and", func(a, b bool) (interface{}, error) {
		return a && b, nil
	})
}

func (jp *JispProgram) Or() error {
	return applyBinaryOp[bool](jp, "or", func(a, b bool) (interface{}, error) {
		return a || b, nil
	})
}

func (jp *JispProgram) Not() error {
	val, err := pop[bool](jp, "not")
	if err != nil {
		return err
	}
	jp.Push(!val)
	return nil
}

func (jp *JispProgram) If(thenBody, elseBody []JispOperation) error {
	conditionVal, err := jp.popValue("if")
	if err != nil {
		return fmt.Errorf("if error: %w", err)
	}
	condition, ok := conditionVal.(bool)
	if !ok {
		return fmt.Errorf("if error: expected boolean condition on stack, got %T", conditionVal)
	}

	if condition {
		return jp.executeOperationsWithPathSegment(thenBody, 0, true)
	} else if elseBody != nil {
		return jp.executeOperationsWithPathSegment(elseBody, 1, true)
	}
	return nil
}

// Try executes the tryBody, and if a JispError occurs, it binds the error message
// to catchVar and executes the catchBody.
func (jp *JispProgram) Try(tryBody []JispOperation, catchVar string, catchBody []JispOperation) error {
	// Execute tryBody. Any errors will be stored in jp.Error.
	if err := jp.executeOperationsWithPathSegment(tryBody, 0, true); err != nil {
		// Propagate control flow signals, but not JispErrors
		return err
	}

	if jp.Error != nil {
		// A JispError occurred, handle it with the catch block.
		caughtErr := jp.Error
		jp.Error = nil // Clear the error before executing the catch body.
		jp.handleCaughtError(caughtErr, catchVar, catchBody, 2)
	}

	return nil
}

// For iterates over a collection (array or object).
// For arrays, it binds each element to loopVar and executes bodyOps.
// For objects, it binds each key to loopVar and executes bodyOps.
func (jp *JispProgram) For(loopVar string, collection interface{}, bodyOps []JispOperation, bodyOpsPathSegment interface{}) error {
	jp.ensureInitialized()

	switch c := collection.(type) {
	case []interface{}:
		for _, item := range c {
			jp.Variables[loopVar] = item
			if err := jp.executeLoopBody(bodyOps, bodyOpsPathSegment); err != nil {
				if errors.Is(err, ErrBreak) {
					return nil // Break from loop
				}
				return err // Propagate other errors
			}
		}
	case map[string]interface{}:
		for key := range c {
			jp.Variables[loopVar] = key
			jp.Variables[loopVar+"_value"] = c[key] // Expose the value
			if err := jp.executeLoopBody(bodyOps, bodyOpsPathSegment); err != nil {
				if errors.Is(err, ErrBreak) {
					return nil // Break from loop
				}
				return err // Propagate other errors
			}
		}
	default:
		return fmt.Errorf("for error: unsupported collection type %T", collection)
	}
	return nil
}

// executeLoopBody runs the operations in a loop's body and handles break/continue.
func (jp *JispProgram) executeLoopBody(bodyOps []JispOperation, bodyOpsPathSegment interface{}) error {
	err := jp.executeOperationsWithPathSegment(bodyOps, bodyOpsPathSegment, true)
	if err != nil {
		if errors.Is(err, ErrContinue) {
			return nil // Signal to continue to next iteration
		}
		return err // Propagate break signals or other errors
	}
	return nil
}

func (jp *JispProgram) handleCaughtError(caughtErr *JispError, catchVar string, catchBody []JispOperation, catchBodyPathSegment interface{}) {
	// Save the error message to the catch variable
	jp.ensureInitialized()
	jp.Variables[catchVar] = caughtErr.Message

	// Execute catchBody
	if catchBody != nil {
		// Errors inside catchBody will be set on jp.Error by ExecuteOperations
		_ = jp.executeOperationsWithPathSegment(catchBody, catchBodyPathSegment, true)
	}
}

// pop pops a single value from the stack and asserts it to the specified type T.
func pop[T any](jp *JispProgram, opName string) (T, error) {
	var zero T // Get the zero value for type T

	if len(jp.Stack) < 1 {
		return zero, fmt.Errorf("stack underflow for %s: expected 1 value", opName)
	}

	val := jp.Stack[len(jp.Stack)-1]
	jp.Stack = jp.Stack[:len(jp.Stack)-1]

	typedVal, ok := val.(T)
	if !ok {
		return zero, fmt.Errorf("%s error: expected a %T on stack, got %T", opName, zero, val)
	}

	return typedVal, nil
}

func (jp *JispProgram) popValue(opName string) (interface{}, error) {
	if len(jp.Stack) < 1 {
		return nil, fmt.Errorf("stack underflow for %s: expected 1 value", opName)
	}
	val := jp.Stack[len(jp.Stack)-1]
	jp.Stack = jp.Stack[:len(jp.Stack)-1]
	return val, nil
}

// popTwo pops two values of the same type T from the stack.
func popTwo[T any](jp *JispProgram, opName string) (T, T, error) {
	var zero T
	if len(jp.Stack) < 2 {
		return zero, zero, fmt.Errorf("stack underflow for %s: expected 2 values", opName)
	}

	b, err := pop[T](jp, opName)
	if err != nil {
		return zero, zero, err
	}

	a, err := pop[T](jp, opName)
	if err != nil {
		return zero, zero, err
	}

	return a, b, nil
}

// popThree pops three values of potentially different types from the stack.
func popThree[T1 any, T2 any, T3 any](jp *JispProgram, opName string) (T1, T2, T3, error) {
	var zero1 T1
	var zero2 T2
	var zero3 T3

	if len(jp.Stack) < 3 {
		return zero1, zero2, zero3, fmt.Errorf("stack underflow for %s: expected 3 values", opName)
	}

	c, err := pop[T3](jp, opName)
	if err != nil {
		return zero1, zero2, zero3, err
	}

	b, err := pop[T2](jp, opName)
	if err != nil {
		return zero1, zero2, zero3, err
	}

	a, err := pop[T1](jp, opName)
	if err != nil {
		return zero1, zero2, zero3, err
	}

	return a, b, c, nil
}

// popx pops n values from the stack and returns them as a slice.

func (jp *JispProgram) popx(opName string, n int) ([]interface{}, error) {
	if len(jp.Stack) < n {
		return nil, fmt.Errorf("stack underflow for %s: expected %d values", opName, n)
	}
	values := jp.Stack[len(jp.Stack)-n:]
	jp.Stack = jp.Stack[:len(jp.Stack)-n]
	return values, nil
}

func (jp *JispProgram) popCollectionOpArgs(opName string, expectedArgs int) (input []interface{}, varName string, ops []JispOperation, err error) {
	args, err := jp.popx(opName, expectedArgs)
	if err != nil {
		return
	}

	input, ok := args[0].([]interface{})
	if !ok {
		err = fmt.Errorf("%s error: expected an array on stack for input, got %T", opName, args[0])
		return
	}

	varName, ok = args[1].(string)
	if !ok {
		err = fmt.Errorf("%s error: expected a string on stack for varName, got %T", opName, args[1])
		return
	}

	ops, err = parseJispOps(args[2])
	if err != nil {
		err = fmt.Errorf("%s error: invalid operations block: %w", opName, err)
		return
	}
	return
}

func (jp *JispProgram) applyStringUnaryOp(opName string, op func(string) string) error {
	val, err := pop[string](jp, opName)
	if err != nil {
		return err
	}
	jp.Push(op(val))
	return nil
}

func applyBinaryOp[T any](jp *JispProgram, opName string, op func(T, T) (interface{}, error)) error {
	a, b, err := popTwo[T](jp, opName)
	if err != nil {
		return err
	}
	res, err := op(a, b)
	if err != nil {
		return fmt.Errorf("%s error: %w", opName, err)
	}
	jp.Push(res)
	return nil
}

func (jp *JispProgram) applyComparisonOp(opName string, opNum func(float64, float64) bool, opStr func(string, string) bool) error {
	vals, err := jp.popx(opName, 2)
	if err != nil {
		return err
	}
	a, b := vals[0], vals[1]

	switch vA := a.(type) {
	case float64:
		vB, ok := b.(float64)
		if !ok {
			return fmt.Errorf("%s error: cannot compare number with %T", opName, b)
		}
		jp.Push(opNum(vA, vB))
	case string:
		vB, ok := b.(string)
		if !ok {
			return fmt.Errorf("%s error: cannot compare string with %T", opName, b)
		}
		jp.Push(opStr(vA, vB))
	default:
		return fmt.Errorf("%s error: unsupported type for comparison: %T", opName, a)
	}
	return nil
}

func parseJispOps(raw interface{}) ([]JispOperation, error) {
	bodyArr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected body to be an array of operations, got %T", raw)
	}
	Ops := make([]JispOperation, len(bodyArr))
	for i, rawOp := range bodyArr {
		opArr, ok := rawOp.([]interface{}) // Expecting each operation to be an array like [opName, arg1, ...]
		if !ok {
			return nil, fmt.Errorf("expected operation to be an array, got %T", rawOp)
		}
		parsedOp, err := parseRawOperation(opArr)
		if err != nil {
			return nil, fmt.Errorf("error parsing operation at index %d: %w", i, err)
		}
		Ops[i] = parsedOp
	}
	return Ops, nil
}

func isTerminal(f *os.File) bool {
	fileInfo, err := f.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func colorizeJSON(data []byte) []byte {
	var result []byte
	inString := false
	for i := 0; i < len(data); i++ {
		char := data[i]

		if inString {
			if char == '"' {
				backslashes := 0
				for k := i - 1; k > 0 && data[k] == '\\'; k-- {
					backslashes++
				}
				if backslashes%2 == 0 {
					inString = false
					result = append(result, char)
					result = append(result, []byte(Reset)...)
					continue
				}
			}
			result = append(result, char)
			continue
		}

		switch {
		case char == '"':
			inString = true
			isKey := false
			j := i + 1
			for j < len(data) {
				if data[j] == '"' {
					backslashes := 0
					for k := j - 1; k > i && data[k] == '\\'; k-- {
						backslashes++
					}
					if backslashes%2 == 0 {
						j++
						break
					}
				}
				j++
			}
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}

			if j < len(data) && data[j] == ':' {
				isKey = true
			}

			if isKey {
				result = append(result, []byte(Green)...)
			} else {
				result = append(result, []byte(Yellow)...)
			}
			result = append(result, char)

		case char == '{' || char == '}' || char == '[' || char == ']':
			result = append(result, []byte(Cyan)...)
			result = append(result, char)
			result = append(result, []byte(Reset)...)
		case (char >= '0' && char <= '9') || char == '-':
			result = append(result, []byte(Magenta)...)
			j := i
			for j < len(data) && ((data[j] >= '0' && data[j] <= '9') || data[j] == '.' || data[j] == 'e' || data[j] == 'E' || data[j] == '+' || data[j] == '-') {
				result = append(result, data[j])
				j++
			}
			result = append(result, []byte(Reset)...)
			i = j - 1
		case bytes.HasPrefix(data[i:], []byte("true")):
			result = append(result, []byte(Blue)...)
			result = append(result, []byte("true")...)
			result = append(result, []byte(Reset)...)
			i += 3
		case bytes.HasPrefix(data[i:], []byte("false")):
			result = append(result, []byte(Blue)...)
			result = append(result, []byte("false")...)
			result = append(result, []byte(Reset)...)
			i += 4
		case bytes.HasPrefix(data[i:], []byte("null")):
			result = append(result, []byte(Red)...)
			result = append(result, []byte("null")...)
			result = append(result, []byte(Reset)...)
			i += 3
		default:
			result = append(result, char)
		}
	}
	return result
}

// ANSI color codes

const (
	Reset   = "\033[0m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
)

func main() {
	log.SetFlags(0) // No timestamps

	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s <jisp_file.json>", os.Args[0])
	}
	filePath := os.Args[1]

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Error reading file: %v", err)
	}

	// This is the core change: unmarshal directly into a JispProgram.
	// This supports loading just `{"code": [...]}` as well as a full state object.
	var jp JispProgram
	if err := json.Unmarshal(fileContent, &jp); err != nil {
		log.Fatalf("Error unmarshaling JISP program: %v", err)
	}

	// Initialize maps and slices if they are nil after unmarshaling
	jp.ensureInitialized()

	if err := jp.processImports(); err != nil {
		log.Fatalf("Error processing imports: %v", err)
	}

	// Start execution only if there's no pre-existing error.
	if jp.Error == nil {
		err = jp.ExecuteFrame(jp.Code, []interface{}{"code"}, false, -1)
		if err != nil && !errors.Is(err, ErrExit) {
			// A non-JispError occurred during execution that wasn't handled.
			// This would be a catastrophic interpreter bug.
			// We wrap it in a JispError for consistent output.
			jp.Error = jp.newError(&JispOperation{Name: "fatal"}, err.Error())
		}
	}

	// Marshal the final state to JSON
	output, err := json.MarshalIndent(&jp, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling final state: %v", err)
	}

	useColor := isTerminal(os.Stdout)
	if useColor {
		output = colorizeJSON(output)
	}

	fmt.Println(string(output))

	if jp.Error != nil {
		os.Exit(1)
	}
}

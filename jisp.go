// Jisp is a stack-based programming system designed for simplicity and integration.
// It uses JSON as its underlying data model for code, stack, and variables,
// making it highly debuggable and interoperable with other tools.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"github.com/xeipuuv/gojsonschema"
)

var (
	ErrBreak    = errors.New("break")
	ErrContinue = errors.New("continue")
	ErrReturn   = errors.New("return")
)

// JispError is a custom error type for JISP program errors.
// It allows the 'try' operation to catch and handle runtime errors gracefully.
type JispError struct {
	OperationName      string        `json:"operation_name"`
	InstructionPointer int           `json:"instruction_pointer"`
	Message            string        `json:"message"`
	StackSnapshot      []interface{} `json:"stack_snapshot"`
}

func (e *JispError) Error() string {
	stackJSON, _ := json.MarshalIndent(e.StackSnapshot, "", "  ")
	return fmt.Sprintf("Jisp Execution Error:\n  Operation: '%s'\n  Instruction: %d\n  Message: %s\n  Stack: %s",
		e.OperationName, e.InstructionPointer, e.Message, stackJSON)
}

// parseRawOperation parses a single operation from a raw array of interfaces.
// It expects the first element to be the operation name (string) and the rest to be arguments.
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
	Ip  int             `json:"Ip"`
	Ops []JispOperation `json:"Ops"`
}

// JispProgram represents the entire state of a JISP program, including the
// execution stack, variables map, a general-purpose state map, and a call stack.
type JispProgram struct {
	Stack      []interface{}          `json:"stack"`
	Variables  map[string]interface{} `json:"variables"`
	State      map[string]interface{} `json:"state"`      // For pop operation target
	Code       []JispOperation        `json:"-"`          // The main program code
	CallStack  []*CallFrame           `json:"call_stack"` // Stack for function calls
}

// currentFrame returns the currently executing frame from the call stack.
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

	Ip := -1
	if frame := jp.currentFrame(); frame != nil {
		Ip = frame.Ip
	}

	return &JispError{
		OperationName:      op.Name,
		InstructionPointer: Ip,
		Message:            message,
		StackSnapshot:      stackCopy,
	}
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

// JispCode represents the code part of a JISP program.
type JispCode struct {
	Code []JispOperation `json:"code"`
}

// operationHandler defines the signature for all JISP operations.
type operationHandler func(jp *JispProgram, op *JispOperation) error

var operations map[string]operationHandler

func init() {
	operations = map[string]operationHandler{
		"push":      	pushOp,
		"pop":       	popOp,
		"set":       	setOp,
		"get":       	getOp,
		"exists":    	existsOp,
		"delete":    	deleteOp,
		"eq":        	eqOp,
		"lt":        	ltOp,
		"gt":        	gtOp,
		"add":       	addOp,
		"sub":       	subOp,
		"mul":       	mulOp,
		"div":       	divOp,
		"mod":       	modOp,
		"and":       	andOp,
		"or":        	orOp,
		"not":       	notOp,
		"if":        	ifOp,
		"while":     	whileOp,
		"trim":      	trimOp,
		"lower":     	lowerOp,
		"upper":     	upperOp,
		"to_string": 	toStringOp,
		"concat":    	concatOp,
		"break":     	breakOp,
		"continue":  	continueOp,
		"len":       	lenOp,
		"keys":      	keysOp,
		"values":    	valuesOp,
		"noop":      	noopOp,
		"try":       	tryOp,
		"replace":   	replaceOp,
		"for":       	forOp,
		"slice":     	sliceOp,
		"raise":     	raiseOp,
		"assert":    	assertOp,
		"range":     	rangeOp,
		"foreach":   	forOp,
		"filter":    	filterOp,
		"map":       	mapOp,
		"reduce":    	reduceOp,
		"sort":      	sortOp,
		"union":     	unionOp,
		"intersection": intersectionOp,
		"difference":   differenceOp,
		"valid":        validOp,
	}
}

// validOp pops a schema and a document from the stack, validates the document against the schema,
// and pushes the boolean result (true for valid, false for invalid) onto the stack.
func validOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("valid error: expected 0 arguments, got %d", len(op.Args))
	}

	docValue, err := jp.popValue("valid")
	if err != nil {
		return fmt.Errorf("valid error: %w", err)
	}

	schemaValue, err := jp.popValue("valid")
	if err != nil {
		return fmt.Errorf("valid error: %w", err)
	}

	// Convert schema and document to gojsonschema Loaders
	schemaLoader := gojsonschema.NewGoLoader(schemaValue)
	documentLoader := gojsonschema.NewGoLoader(docValue)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("valid error during schema validation: %w", err)
	}

	jp.Push(result.Valid())
	return nil
}

// Helper to convert []interface{} to []comparable for set operations
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

// Helper to get unique elements from a slice
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

func popTwoComparableSlices(jp *JispProgram, opName string) ([]interface{}, []interface{}, error) {
	a2, err := pop[[]interface{}](jp, opName)
	if err != nil {
		return nil, nil, err
	}
	a1, err := pop[[]interface{}](jp, opName)
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

// unionOp performs the union of two arrays on the stack.
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

// intersectionOp performs the intersection of two arrays on the stack.
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

// differenceOp performs the set difference (a1 - a2) of two arrays on the stack.
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
		// Attempt to sort as numbers or strings
		if len(v) == 0 {
			jp.Push(v) // Push empty slice back
			return nil
		}

		// Check if all elements are numbers
		allNumbers := true
		for _, item := range v {
			if _, ok := item.(float64); !ok {
				allNumbers = false
				break
			}
		}

		if allNumbers {
			numSlice := make([]float64, len(v))
			for i, item := range v {
				numSlice[i] = item.(float64)
			}
			sort.Float64s(numSlice)
			result := make([]interface{}, len(numSlice))
			for i, num := range numSlice {
				result[i] = num
			}
			jp.Push(result)
			return nil
		}

		// Check if all elements are strings
		allStrings := true
		for _, item := range v {
			if _, ok := item.(string); !ok {
				allStrings = false
				break
			}
		}

		if allStrings {
			strSlice := make([]string, len(v))
			for i, item := range v {
				strSlice[i] = item.(string)
			}
			sort.Strings(strSlice)
			result := make([]interface{}, len(strSlice))
			for i, str := range strSlice {
				result[i] = str
			}
			jp.Push(result)
			return nil
		}

		return fmt.Errorf("sort error: array contains mixed types or unsortable types")

	default:
		return fmt.Errorf("sort error: unsupported type %T for sorting, expected array", val)
	}
}

func reduceOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("reduce error: expected 0 arguments, got %d", len(op.Args))
	}

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
		jp.Push(accumulator) // Push current accumulator onto stack
		jp.Push(item)        // Push current item onto stack

		previousStackLen := len(jp.Stack) // Store stack length before executing reduceOps

		if err := jp.ExecuteOperations(reduceOps); err != nil {
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

	args, err := jp.popx("map", 3)
	if err != nil {
		return err
	}

	input, ok := args[0].([]interface{})
	if !ok {
		return fmt.Errorf("map error: expected an array on stack for input, got %T", args[0])
	}

	varName, ok := args[1].(string)
	if !ok {
		return fmt.Errorf("map error: expected a string on stack for varName, got %T", args[1])
	}

	mapOps, err := parseJispOps(args[2])
	if err != nil {
		return fmt.Errorf("map error: invalid operations block: %w", err)
	}

	var result []interface{}
	for _, item := range input {
		jp.Variables[varName] = item
		if err := jp.ExecuteOperations(mapOps); err != nil {
			return err
		}
		res, err := jp.popValue("map")
		if err != nil {
			return err
		}
		result = append(result, res)
	}

	jp.Push(result)
	return nil
}

func filterOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("filter error: expected 0 arguments, got %d", len(op.Args))
	}

	args, err := jp.popx("filter", 3)
	if err != nil {
		return err
	}

	input, ok := args[0].([]interface{})
	if !ok {
		return fmt.Errorf("filter error: expected an array on stack for input, got %T", args[0])
	}

	varName, ok := args[1].(string)
	if !ok {
		return fmt.Errorf("filter error: expected a string on stack for varName, got %T", args[1])
	}

	conditionOps, err := parseJispOps(args[2])
	if err != nil {
		return fmt.Errorf("filter error: invalid condition block: %w", err)
	}

	var result []interface{}
	for _, item := range input {
		jp.Variables[varName] = item
		if err := jp.ExecuteOperations(conditionOps); err != nil {
			return err
		}
		condition, err := pop[bool](jp, "filter")
		if err != nil {
			return err
		}
		if condition {
			result = append(result, item)
		}
	}

	jp.Push(result)
	return nil
}

func rangeOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 {
		return fmt.Errorf("range error: expected 0 arguments, got %d", len(op.Args))
	}

	args, err := jp.popx("range", 3)
	if err != nil {
		return err
	}

	start, okStart := args[0].(float64)
	stop, okStop := args[1].(float64)
	step, okStep := args[2].(float64)

	if !okStart || !okStop || !okStep {
		return fmt.Errorf("range error: all arguments on stack must be numbers")
	}

	var result []float64
	for i := start; i < stop; i += step {
		result = append(result, i)
	}
	jp.Push(result)
	return nil
}

func raiseOp(jp *JispProgram, _ *JispOperation) error {
	errMsg, err := jp.popString("raise")
	if err != nil {
		return err
	}
	return &JispError{Message: errMsg}
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
		errMsg := "assertion failed"
		if len(op.Args) > 0 {
			if customMsg, ok := op.Args[0].(string); ok {
				errMsg = customMsg
			}
		}
		return &JispError{Message: errMsg}
	}

	return nil
}

func noopOp(jp *JispProgram, _ *JispOperation) error {
	// No operation, do nothing.
	return nil
}

func sliceOp(jp *JispProgram, _ *JispOperation) error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("slice error: stack underflow, expected at least 2 values (input, start)")
	}

	var inputVal, startRaw, endRaw interface{}
	val2, val1 := jp.Stack[len(jp.Stack)-1], jp.Stack[len(jp.Stack)-2]
	_, isNum1 := val1.(float64)
	_, isNum2 := val2.(float64)

	if len(jp.Stack) >= 3 && isNum1 && isNum2 {
		endRaw, startRaw, inputVal = jp.Stack[len(jp.Stack)-1], jp.Stack[len(jp.Stack)-2], jp.Stack[len(jp.Stack)-3]
		jp.Stack = jp.Stack[:len(jp.Stack)-3]
	} else {
		startRaw, inputVal = jp.Stack[len(jp.Stack)-1], jp.Stack[len(jp.Stack)-2]
		jp.Stack = jp.Stack[:len(jp.Stack)-2]
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

	var sliceable Slicer
	switch v := inputVal.(type) {
	case string:
		sliceable = stringSlicer(v)
	case []interface{}:
		sliceable = sliceSlicer(v)
	default:
		return fmt.Errorf("slice error: unsupported type %T for slicing, expected string or array", inputVal)
	}

	length := sliceable.Len()
	if !hasEnd {
		end = length
	}

	if start < 0 || end < start || end > length {
		return fmt.Errorf("slice error: invalid indices [%d:%d] for collection of length %d", start, end, length)
	}

	jp.Push(sliceable.Slice(start, end))
	return nil
}

// Slicer defines an interface for types that can be sliced.
type Slicer interface {
	Len() int
	Slice(i, j int) interface{}
}

type stringSlicer string

func (s stringSlicer) Len() int                   { return len(s) }
func (s stringSlicer) Slice(i, j int) interface{} { return s[i:j] }

type sliceSlicer []interface{}

func (s sliceSlicer) Len() int                   { return len(s) }
func (s sliceSlicer) Slice(i, j int) interface{} { return s[i:j] }

// ExecuteOperations pushes a new call frame for the given operations and executes them.
// It manages the instruction pointer within this frame and handles control flow.
func (jp *JispProgram) ExecuteOperations(ops []JispOperation) error {
	if len(ops) == 0 {
		return nil
	}
	frame := &CallFrame{Ops: ops, Ip: 0}
	jp.CallStack = append(jp.CallStack, frame)

	// Defer popping the frame. This ensures that the call stack is cleaned up
	// correctly, whether the function returns normally or due to an error.
	defer func() {
		if len(jp.CallStack) > 0 && jp.CallStack[len(jp.CallStack)-1] == frame {
			jp.CallStack = jp.CallStack[:len(jp.CallStack)-1]
		}
	}()

	for frame.Ip < len(frame.Ops) {
		op := frame.Ops[frame.Ip]

		handler, found := operations[op.Name]
		if !found {
			return jp.newError(&op, fmt.Sprintf("unknown operation: %s", op.Name))
		}

		if err := handler(jp, &op); err != nil {
			var jispErr *JispError
			switch {
			case errors.Is(err, ErrBreak), errors.Is(err, ErrContinue), errors.Is(err, ErrReturn):
				return err // Propagate control flow signals directly
			case errors.As(err, &jispErr):
				return err // Already a JispError, propagate
			default:
				// Wrap other errors as JispError for 'try' to catch
				return jp.newError(&op, err.Error())
			}
		}
		frame.Ip++
	}
	return nil
}

// --- Operation Handlers ---

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

func breakOp(jp *JispProgram, _ *JispOperation) error {
	return ErrBreak
}

func continueOp(jp *JispProgram, _ *JispOperation) error {
	return ErrContinue
}

func setOp(jp *JispProgram, _ *JispOperation) error    { return jp.Set() }
func getOp(jp *JispProgram, _ *JispOperation) error    { return jp.Get() }
func existsOp(jp *JispProgram, _ *JispOperation) error { return jp.Exists() }
func deleteOp(jp *JispProgram, _ *JispOperation) error { return jp.Delete() }
func eqOp(jp *JispProgram, _ *JispOperation) error     { return jp.Eq() }
func ltOp(jp *JispProgram, _ *JispOperation) error     { return jp.Lt() }
func gtOp(jp *JispProgram, _ *JispOperation) error     { return jp.Gt() }
func addOp(jp *JispProgram, _ *JispOperation) error    { return jp.Add() }
func subOp(jp *JispProgram, _ *JispOperation) error    { return jp.Sub() }
func mulOp(jp *JispProgram, _ *JispOperation) error    { return jp.Mul() }
func divOp(jp *JispProgram, _ *JispOperation) error    { return jp.Div() }
func modOp(jp *JispProgram, _ *JispOperation) error    { return jp.Mod() }
func andOp(jp *JispProgram, _ *JispOperation) error    { return jp.And() }
func orOp(jp *JispProgram, _ *JispOperation) error     { return jp.Or() }
func notOp(jp *JispProgram, _ *JispOperation) error    { return jp.Not() }

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

	collection := op.Args[1]

	bodyOps, err := parseJispOps(op.Args[2])
	if err != nil {
		return fmt.Errorf("for error in 'body_operations': %w", err)
	}

	return jp.For(loopVar, collection, bodyOps)
}

func trimOp(jp *JispProgram, _ *JispOperation) error {
	return jp.applyStringUnaryOp("trim", strings.TrimSpace)
}

func lowerOp(jp *JispProgram, _ *JispOperation) error {
	return jp.applyStringUnaryOp("lower", strings.ToLower)
}

func upperOp(jp *JispProgram, _ *JispOperation) error {
	return jp.applyStringUnaryOp("upper", strings.ToUpper)
}

func toStringOp(jp *JispProgram, _ *JispOperation) error {
	val, err := jp.popValue("to_string")
	if err != nil {
		return err
	}
	jp.Push(fmt.Sprintf("%v", val))
	return nil
}

func concatOp(jp *JispProgram, _ *JispOperation) error {
	val2, err := jp.popString("concat")
	if err != nil {
		return err
	}
	val1, err := jp.popString("concat")
	if err != nil {
		return err
	}
	jp.Push(val1 + val2)
	return nil
}

func replaceOp(jp *JispProgram, _ *JispOperation) error {
	values, err := jp.popx("replace", 3)
	if err != nil {
		return err
	}
	str, okStr := values[0].(string)
	old, okOld := values[1].(string)
	new, okNew := values[2].(string)
	if !okStr || !okOld || !okNew {
		return fmt.Errorf("replace error: expected three strings on the stack")
	}
	jp.Push(strings.ReplaceAll(str, old, new))
	return nil
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
		// Push the condition path and get its value
		jp.Push(conditionPath)
		if err := jp.Get(); err != nil {
			return fmt.Errorf("while error: failed to get condition variable '%s': %w", conditionPath, err)
		}

		conditionVal, err := jp.popValue("while condition check")
		if err != nil {
			return fmt.Errorf("while error: %w", err)
		}

		condition, ok := conditionVal.(bool)
		if !ok {
			return fmt.Errorf("while error: expected boolean condition at '%s', got %T", conditionPath, conditionVal)
		}

		if !condition {
			break
		}

		if err := jp.ExecuteOperations(bodyOps); err != nil {
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

func lenOp(jp *JispProgram, op *JispOperation) error {
	return applyCollectionOp(jp, "len", op, collectionHandlers{
		stringHandler: func(s string) (interface{}, error) {
			return float64(len(s)), nil
		},
		arrayHandler: func(a []interface{}) (interface{}, error) {
			return float64(len(a)), nil
		},
		objectHandler: func(m map[string]interface{}) (interface{}, error) {
			return float64(len(m)), nil
		},
	})
}

func valuesOp(jp *JispProgram, op *JispOperation) error {
	return applyCollectionOp(jp, "values", op, collectionHandlers{
		objectHandler: func(m map[string]interface{}) (interface{}, error) {
			values := make([]interface{}, 0, len(m))
			for _, val := range m {
				values = append(values, val)
			}
			return values, nil
		},
	})
}

func keysOp(jp *JispProgram, op *JispOperation) error {
	return applyCollectionOp(jp, "keys", op, collectionHandlers{
		objectHandler: func(m map[string]interface{}) (interface{}, error) {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			return keys, nil
		},
	})
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

// --- Core JISP Logic ---

// Push adds a value to the top of the stack.
func (jp *JispProgram) Push(value interface{}) {
	jp.Stack = append(jp.Stack, value)
}

// Pop removes the top value from the stack and moves it to the program state field specified by fieldName.
func (jp *JispProgram) Pop(fieldName string) error {
	value, err := jp.popValue("pop")
	if err != nil {
		return err
	}
	if jp.State == nil {
		jp.State = make(map[string]interface{})
	}
	jp.State[fieldName] = value
	return nil
}

// Set stores a value from the stack into the Variables map using a key from the stack.
func (jp *JispProgram) Set() error {
	key, val, err := jp.popKeyValue("set")
	if err != nil {
		return err
	}
	if jp.Variables == nil {
		jp.Variables = make(map[string]interface{})
	}
	// Note: In `popKeyValue`, the key is popped first, then the value,
	// which matches the expected stack order `[..., value, key]`.
	jp.Variables[key] = val
	return nil
}

func (jp *JispProgram) getValueByPath(path []interface{}) (interface{}, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("get error: path array cannot be empty")
	}

	rootKey, ok := path[0].(string)
	if !ok {
		return nil, fmt.Errorf("get error: first element of path must be a string variable name, got %T", path[0])
	}

	currentVal, found := jp.Variables[rootKey]
	if !found {
		return nil, fmt.Errorf("get error: variable '%s' not found", rootKey)
	}

	for i := 1; i < len(path); i++ {
		segment := path[i]
		switch key := segment.(type) {
		case string:
			if m, ok := currentVal.(map[string]interface{}); ok {
				if val, found := m[key]; found {
					currentVal = val
				} else {
					return nil, fmt.Errorf("get error: key '%s' not found in path %v", key, path)
				}
			} else {
				return nil, fmt.Errorf("get error: trying to access non-map with string key '%s' in path %v", key, path)
			}
		case float64:
			index := int(key)
			if a, ok := currentVal.([]interface{}); ok {
				if index >= 0 && index < len(a) {
					currentVal = a[index]
				} else {
					return nil, fmt.Errorf("get error: index %d out of bounds for path %v", index, path)
				}
			} else {
				return nil, fmt.Errorf("get error: trying to access non-array with numeric index %d in path %v", index, path)
			}
		default:
			return nil, fmt.Errorf("get error: invalid path segment type %T in path %v", segment, path)
		}
	}
	return currentVal, nil
}

// Get retrieves a value from the Variables map and pushes it onto the stack.
// The key can be a string for a top-level variable, or an array for a nested value.
func (jp *JispProgram) Get() error {
	pathVal, err := jp.popValue("get")
	if err != nil {
		return err
	}

	switch path := pathVal.(type) {
	case string:
		val, found := jp.Variables[path]
		if !found {
			return fmt.Errorf("get error: variable '%s' not found", path)
		}
		jp.Push(val)
		return nil
	case []interface{}:
		val, err := jp.getValueByPath(path)
		if err != nil {
			return err
		}
		jp.Push(val)
		return nil
	default:
		return fmt.Errorf("get error: expected a string or an array path on stack, got %T", pathVal)
	}
}

// Exists checks if a variable exists and pushes the boolean result onto the stack.
func (jp *JispProgram) Exists() error {
	key, err := pop[string](jp, "exists")
	if err != nil {
		return err
	}
	_, found := jp.Variables[key]
	jp.Push(found)
	return nil
}

// Delete removes a variable from the Variables map.
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

// Eq pops two values, checks for strict equality, and pushes the boolean result.
func (jp *JispProgram) Eq() error {
	vals, err := jp.popx("eq", 2)
	if err != nil {
		return err
	}
	jp.Push(vals[0] == vals[1])
	return nil
}

// Lt pops two values, checks if the first is less than the second, and pushes the boolean result.
func (jp *JispProgram) Lt() error {
	return jp.applyComparisonOp("lt",
		func(a, b float64) bool { return a < b },
		func(a, b string) bool { return a < b },
	)
}

// Gt pops two values, checks if the first is greater than the second, and pushes the boolean result.
func (jp *JispProgram) Gt() error {
	return jp.applyComparisonOp("gt",
		func(a, b float64) bool { return a > b },
		func(a, b string) bool { return a > b },
	)
}

// Add pops two numbers, adds them, and pushes the result.
func (jp *JispProgram) Add() error {
	return applyBinaryOp[float64](jp, "add", func(a, b float64) (interface{}, error) {
		return a + b, nil
	})
}

// Sub pops two numbers, subtracts them, and pushes the result.
func (jp *JispProgram) Sub() error {
	return applyBinaryOp[float64](jp, "sub", func(a, b float64) (interface{}, error) {
		return a - b, nil
	})
}

// Mul pops two numbers, multiplies them, and pushes the result.
func (jp *JispProgram) Mul() error {
	return applyBinaryOp[float64](jp, "mul", func(a, b float64) (interface{}, error) {
		return a * b, nil
	})
}

// Div pops two numbers, divides them, and pushes the result.
func (jp *JispProgram) Div() error {
	return applyBinaryOp[float64](jp, "div", func(a, b float64) (interface{}, error) {
		if b == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return a / b, nil
	})
}

// Mod pops two numbers, performs modulo, and pushes the result.
func (jp *JispProgram) Mod() error {
	return applyBinaryOp[float64](jp, "mod", func(a, b float64) (interface{}, error) {
		if b == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return math.Mod(a, b), nil
	})
}

// And pops two booleans, performs logical AND, and pushes the result.
func (jp *JispProgram) And() error {
	return applyBinaryOp[bool](jp, "and", func(a, b bool) (interface{}, error) {
		return a && b, nil
	})
}

// Or pops two booleans, performs logical OR, and pushes the result.
func (jp *JispProgram) Or() error {
	return applyBinaryOp[bool](jp, "or", func(a, b bool) (interface{}, error) {
		return a || b, nil
	})
}

// Not pops a boolean, performs logical NOT, and pushes the result.
func (jp *JispProgram) Not() error {
	val, err := pop[bool](jp, "not")
	if err != nil {
		return err
	}
	jp.Push(!val)
	return nil
}

// If conditionally executes operations based on a boolean popped from the stack.
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
		return jp.ExecuteOperations(thenBody)
	} else if elseBody != nil {
		return jp.ExecuteOperations(elseBody)
	}
	return nil
}

// Try executes the tryBody, and if a JispError occurs, it binds the error message
// to catchVar and executes the catchBody.
func (jp *JispProgram) Try(tryBody []JispOperation, catchVar string, catchBody []JispOperation) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// This catches panics that are not JispError.
			// Re-throw if it's not a JispError, or if catchBody is not provided.
			if _, ok := r.(*JispError); !ok || catchBody == nil {
				panic(r)
			}
			// If it's a JispError and catchBody exists, handle it.
			err = jp.handleCaughtError(r, catchVar, catchBody)
		}
	}()

	// Execute tryBody
	if tryErr := jp.ExecuteOperations(tryBody); tryErr != nil {
		if jispErr, ok := tryErr.(*JispError); ok {
			// JispError occurred, handle it with the catch block
			return jp.handleCaughtError(jispErr, catchVar, catchBody)
		}
		return tryErr // Propagate other types of errors
	}
	return nil
}

// For iterates over a collection (array or object).
// For arrays, it binds each element to loopVar and executes bodyOps.
// For objects, it binds each key to loopVar and executes bodyOps.
func (jp *JispProgram) For(loopVar string, collection interface{}, bodyOps []JispOperation) error {
	if jp.Variables == nil {
		jp.Variables = make(map[string]interface{})
	}

	switch c := collection.(type) {
	case []interface{}:
		for _, item := range c {
			jp.Variables[loopVar] = item
			if err := jp.executeLoopBody(bodyOps); err != nil {
				if errors.Is(err, ErrBreak) {
					return nil // Break from loop
				}
				return err // Propagate other errors
			}
		}
	case map[string]interface{}:
		for key := range c {
			jp.Variables[loopVar] = key
			if err := jp.executeLoopBody(bodyOps); err != nil {
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
func (jp *JispProgram) executeLoopBody(bodyOps []JispOperation) error {
	err := jp.ExecuteOperations(bodyOps)
	if err != nil {
		if errors.Is(err, ErrContinue) {
			return nil // Signal to continue to next iteration
		}
		return err // Propagate break signals or other errors
	}
	return nil
}

func (jp *JispProgram) handleCaughtError(caught interface{}, catchVar string, catchBody []JispOperation) error {
	var errMsg string
	if jispErr, ok := caught.(*JispError); ok {
		errMsg = jispErr.Message
	} else if err, ok := caught.(error); ok {
		errMsg = err.Error()
	} else {
		errMsg = fmt.Sprintf("%v", caught)
	}

	// Save the error message to the catch variable
	if jp.Variables == nil {
		jp.Variables = make(map[string]interface{})
	}
	jp.Variables[catchVar] = errMsg

	// Execute catchBody
	if catchBody != nil {
		return jp.ExecuteOperations(catchBody)
	}
	return nil // If no catchBody, just absorb the error
}

// --- Helper Functions ---

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

func (jp *JispProgram) popTwoValues(opName string) (interface{}, interface{}, error) {
	if len(jp.Stack) < 2 {
		return nil, nil, fmt.Errorf("stack underflow for %s: expected 2 values", opName)
	}
	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2]
	return a, b, nil
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

// popx pops n values from the stack and returns them as a slice.
func (jp *JispProgram) popx(opName string, n int) ([]interface{}, error) {
	if len(jp.Stack) < n {
		return nil, fmt.Errorf("stack underflow for %s: expected %d values", opName, n)
	}
	values := jp.Stack[len(jp.Stack)-n:]
	jp.Stack = jp.Stack[:len(jp.Stack)-n]
	return values, nil
}

func (jp *JispProgram) popString(opName string) (string, error) {
	return pop[string](jp, opName)
}

func (jp *JispProgram) popKeyValue(opName string) (string, interface{}, error) {
	values, err := jp.popx(opName, 2)
	if err != nil {
		return "", nil, err
	}
	value := values[0]
	key, ok := values[1].(string)
	if !ok {
		return "", nil, fmt.Errorf("%s error: expected a string key on stack, got %T", opName, values[1])
	}
	return key, value, nil
}

func (jp *JispProgram) applyStringUnaryOp(opName string, op func(string) string) error {
	val, err := jp.popString(opName)
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

func main() {
	var programData map[string]interface{}
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&programData); err != nil {
		log.Fatalf("Error reading JISP program from stdin: %v", err)
	}

	// Extract code
	rawCode, ok := programData["code"]
	if !ok {
		log.Fatalf("Input JSON must have a 'code' field.")
	}
	codeOps, err := parseJispOps(rawCode)
	if err != nil {
		log.Fatalf("Error parsing 'code' field: %v", err)
	}

	// Initialize JispProgram with references to the programData map
	jp := &JispProgram{
		Code: codeOps,
	}

	// Initialize stack
	if stack, ok := programData["stack"].([]interface{}); ok {
		jp.Stack = stack
	} else {
		jp.Stack = []interface{}{}
		programData["stack"] = jp.Stack
	}

	// Initialize variables
	if variables, ok := programData["variables"].(map[string]interface{}); ok {
		jp.Variables = variables
	} else {
		jp.Variables = make(map[string]interface{})
		programData["variables"] = jp.Variables
	}

	// Initialize state
	if state, ok := programData["state"].(map[string]interface{}); ok {
		jp.State = state
	} else {
		jp.State = make(map[string]interface{})
		programData["state"] = jp.State
	}

	// Initialize call stack
	jp.CallStack = []*CallFrame{
		{
			Ip:  0,
			Ops: jp.Code,
		},
	}
	programData["call_stack"] = jp.CallStack

	executionErr := jp.ExecuteOperations(jp.Code)

	// Update the map with the final state of mutable fields
	programData["stack"] = jp.Stack
	programData["variables"] = jp.Variables
	programData["state"] = jp.State
	programData["call_stack"] = jp.CallStack

	if executionErr != nil {
		var jispErr *JispError
		if errors.As(executionErr, &jispErr) {
			programData["error"] = jispErr
		} else {
			programData["error"] = map[string]string{"message": executionErr.Error()}
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(programData); err != nil {
		log.Fatalf("Error encoding JISP program state to stdout: %v", err)
	}

	if executionErr != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

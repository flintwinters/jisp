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
	"strings"
)

// Custom error types for control flow
type BreakSignal struct{}

func (b *BreakSignal) Error() string { return "break" }

type ContinueSignal struct{}

func (c *ContinueSignal) Error() string { return "continue" }

// JispError is a custom error type for JISP program errors.
// It allows the 'try' operation to catch and handle runtime errors gracefully.
type JispError struct {
	OperationName      string
	InstructionPointer int
	Message            string
	StackSnapshot      []interface{}
}

func (e *JispError) Error() string {
	stackJSON, _ := json.MarshalIndent(e.StackSnapshot, "", "  ")
	return fmt.Sprintf("Jisp Execution Error:\n  Operation: '%s'\n  Instruction: %d\n  Message: %s\n  Stack:\n%s",
		e.OperationName, e.InstructionPointer, e.Message, stackJSON)
}

// JispProgram represents the entire state of a JISP program, including the
// execution stack, variables map, and a general-purpose state map.
type JispProgram struct {
	Stack     []interface{}          `json:"stack"`
	Variables map[string]interface{} `json:"variables"`
	State     map[string]interface{} `json:"state"` // For pop operation target
	ip        int                    // Instruction pointer
}

// newError creates a new JispError with the current program state.
func (jp *JispProgram) newError(op *JispOperation, message string) *JispError {
	stackCopy := make([]interface{}, len(jp.Stack))
	copy(stackCopy, jp.Stack)
	return &JispError{
		OperationName:      op.Name,
		InstructionPointer: jp.ip,
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

func (op *JispOperation) UnmarshalJSON(data []byte) error {
	var raw []interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("JispOperation UnmarshalJSON: expected an array, got %w", err)
	}

	if len(raw) == 0 {
		return fmt.Errorf("JispOperation UnmarshalJSON: array is empty")
	}

	opName, ok := raw[0].(string)
	if !ok {
		return fmt.Errorf("JispOperation UnmarshalJSON: first element (operation name) is not a string, got %T", raw[0])
	}
	op.Name = opName

	if len(raw) > 1 {
		op.Args = raw[1:]
	} else {
		op.Args = []interface{}{}
	}

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
		"push":      pushOp,
		"pop":       popOp,
		"set":       setOp,
		"get":       getOp,
		"exists":    existsOp,
		"delete":    deleteOp,
		"eq":        eqOp,
		"lt":        ltOp,
		"gt":        gtOp,
		"add":       addOp,
		"sub":       subOp,
		"mul":       mulOp,
		"div":       divOp,
		"mod":       modOp,
		"and":       andOp,
		"or":        orOp,
		"not":       notOp,
		"if":        ifOp,
		"while":     whileOp,
		"trim":      trimOp,
		"lower":     lowerOp,
		"upper":     upperOp,
		"to_string": toStringOp,
		"concat":    concatOp,
		"break":     breakOp,
		"continue":  continueOp,
		"len":       lenOp,
		"keys":      keysOp,
		"values":    valuesOp,
		"noop":      noopOp,
		"try":       tryOp,
		"replace":   replaceOp,
		"for":       forOp,
		"slice":     sliceOp,
		"raise":     raiseOp,
		"assert":    assertOp,
		"range":     rangeOp,
		"foreach":   forOp,
	}
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

// ExecuteOperations iterates and executes a slice of JispOperations.
func (jp *JispProgram) ExecuteOperations(ops []JispOperation) error {
	for i, op := range ops {
		jp.ip = i
		handler, found := operations[op.Name]
		if !found {
			return jp.newError(&op, fmt.Sprintf("unknown operation: %s", op.Name))
		}
		if err := handler(jp, &op); err != nil {
			var breakSig *BreakSignal
			var contSig *ContinueSignal
			var jispErr *JispError

			switch {
			case errors.As(err, &breakSig), errors.As(err, &contSig):
				return err // Propagate control flow signals directly
			case errors.As(err, &jispErr):
				return err // Already a JispError, propagate
			default:
				// Wrap other errors as JispError for 'try' to catch
				return jp.newError(&op, err.Error())
			}
		}
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
	return &BreakSignal{}
}

func continueOp(jp *JispProgram, _ *JispOperation) error {
	return &ContinueSignal{}
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
			if _, isBreak := err.(*BreakSignal); isBreak {
				break // Exit the while loop
			}
			if _, isContinue := err.(*ContinueSignal); isContinue {
				continue // Skip to the next iteration of the while loop
			}
			return fmt.Errorf("while error during body execution: %w", err)
		}
	}
	return nil
}

func lenOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 { // len operation takes its argument from the stack, not from op.Args
		return fmt.Errorf("len error: expected 0 arguments, got %d", len(op.Args))
	}

	val, err := jp.popValue("len")
	if err != nil {
		return fmt.Errorf("len error: %w", err)
	}

	var length float64
	switch v := val.(type) {
	case string:
		length = float64(len(v))
	case []interface{}: // Array
		length = float64(len(v))
	case map[string]interface{}: // Object
		length = float64(len(v))
	default:
		return fmt.Errorf("len error: unsupported type %T", val)
	}

	jp.Push(length)
	return nil
}

func valuesOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 { // values operation takes its argument from the stack, not from op.Args
		return fmt.Errorf("values error: expected 0 arguments, got %d", len(op.Args))
	}

	val, err := jp.popValue("values")
	if err != nil {
		return fmt.Errorf("values error: %w", err)
	}

	var values []interface{}
	switch v := val.(type) {
	case map[string]interface{}: // Object
		for _, val := range v {
			values = append(values, val)
		}
	default:
		return fmt.Errorf("values error: unsupported type %T, expected object", val)
	}

	jp.Push(values)
	return nil
}

func keysOp(jp *JispProgram, op *JispOperation) error {
	if len(op.Args) != 0 { // keys operation takes its argument from the stack, not from op.Args
		return fmt.Errorf("keys error: expected 0 arguments, got %d", len(op.Args))
	}

	val, err := jp.popValue("keys")
	if err != nil {
		return fmt.Errorf("keys error: %w", err)
	}

	var keys []string
	switch v := val.(type) {
	case map[string]interface{}: // Object
		for k := range v {
			keys = append(keys, k)
		}
	default:
		return fmt.Errorf("keys error: unsupported type %T, expected object", val)
	}

	jp.Push(keys)
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

// Get retrieves a value from the Variables map and pushes it onto the stack.
func (jp *JispProgram) Get() error {
	key, err := pop[string](jp, "get")
	if err != nil {
		return err
	}
	val, found := jp.Variables[key]
	if !found {
		return fmt.Errorf("get error: variable '%s' not found", key)
	}
	jp.Push(val)
	return nil
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
	return jp.applyNumericBinaryOp("add", func(a, b float64) (interface{}, error) {
		return a + b, nil
	})
}

// Sub pops two numbers, subtracts them, and pushes the result.
func (jp *JispProgram) Sub() error {
	return jp.applyNumericBinaryOp("sub", func(a, b float64) (interface{}, error) {
		return a - b, nil
	})
}

// Mul pops two numbers, multiplies them, and pushes the result.
func (jp *JispProgram) Mul() error {
	return jp.applyNumericBinaryOp("mul", func(a, b float64) (interface{}, error) {
		return a * b, nil
	})
}

// Div pops two numbers, divides them, and pushes the result.
func (jp *JispProgram) Div() error {
	return jp.applyNumericBinaryOp("div", func(a, b float64) (interface{}, error) {
		if b == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return a / b, nil
	})
}

// Mod pops two numbers, performs modulo, and pushes the result.
func (jp *JispProgram) Mod() error {
	return jp.applyNumericBinaryOp("mod", func(a, b float64) (interface{}, error) {
		if b == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return math.Mod(a, b), nil
	})
}

// And pops two booleans, performs logical AND, and pushes the result.
func (jp *JispProgram) And() error {
	return jp.applyBooleanBinaryOp("and", func(a, b bool) bool { return a && b })
}

// Or pops two booleans, performs logical OR, and pushes the result.
func (jp *JispProgram) Or() error {
	return jp.applyBooleanBinaryOp("or", func(a, b bool) bool { return a || b })
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
				if _, isBreak := err.(*BreakSignal); isBreak {
					return nil // Break from loop
				}
				return err // Propagate other errors
			}
		}
	case map[string]interface{}:
		for key := range c {
			jp.Variables[loopVar] = key
			if err := jp.executeLoopBody(bodyOps); err != nil {
				if _, isBreak := err.(*BreakSignal); isBreak {
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
		if _, isContinue := err.(*ContinueSignal); isContinue {
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
	if len(jp.Stack) < 2 {
		return "", nil, fmt.Errorf("stack underflow for %s: expected value and key on stack", opName)
	}
	// The key is popped first because it's on top of the value.
	key, err := pop[string](jp, opName)
	if err != nil {
		return "", nil, err
	}
	value, err := jp.popValue(opName)
	if err != nil {
		return "", nil, err
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

func (jp *JispProgram) applyNumericBinaryOp(opName string, op func(float64, float64) (interface{}, error)) error {
	a, b, err := popTwo[float64](jp, opName)
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

func (jp *JispProgram) applyBooleanBinaryOp(opName string, op func(bool, bool) bool) error {
	a, b, err := popTwo[bool](jp, opName)
	if err != nil {
		return err
	}
	jp.Push(op(a, b))
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
	ops := make([]JispOperation, len(bodyArr))
	for i, rawOp := range bodyArr {
		opArr, ok := rawOp.([]interface{})
		if !ok {
			return nil, fmt.Errorf("expected operation to be an array, got %T", rawOp)
		}
		if len(opArr) == 0 {
			return nil, fmt.Errorf("operation array is empty")
		}

		opName, ok := opArr[0].(string)
		if !ok {
			return nil, fmt.Errorf("operation name is not a string, got %T", opArr[0])
		}

		ops[i].Name = opName
		if len(opArr) > 1 {
			ops[i].Args = opArr[1:]
		} else {
			ops[i].Args = []interface{}{}
		}
	}
	return ops, nil
}

func main() {
	jp := &JispProgram{
		Stack:     []interface{}{},
		Variables: make(map[string]interface{}),
		State:     make(map[string]interface{}),
	}

	var jispCode JispCode
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&jispCode); err != nil {
		log.Fatalf("Error reading JISP program from stdin: %v", err)
	}

	if err := jp.ExecuteOperations(jispCode.Code); err != nil {
		log.Fatalf("Error during program execution: %v", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jp); err != nil {
		log.Fatalf("Error encoding JISP program state to stdout: %v", err)
	}
	os.Exit(0)
}
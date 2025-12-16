package main

import (
	"encoding/json"
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

// JispProgram represents the entire state of a JISP program.
type JispProgram struct {
	Stack     []interface{}          `json:"stack"`
	Variables map[string]interface{} `json:"variables"`
	State     map[string]interface{} `json:"state"` // For pop operation target
}

// JispOperation represents a single operation in a JISP program.
type JispOperation struct {
	Name string        `json:"op_name"` // Not directly from JSON, but will be set by UnmarshalJSON
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
		"push":   pushOp,
		"pop":    popOp,
		"set":    setOp,
		"get":    getOp,
		"exists": existsOp,
		"delete": deleteOp,
		"eq":     eqOp,
		"lt":     ltOp,
		"gt":     gtOp,
		"add":    addOp,
		"sub":    subOp,
		"mul":    mulOp,
		"div":    divOp,
		"mod":    modOp,
		"and":    andOp,
		"or":     orOp,
		"not":    notOp,
		"if":      ifOp,
		"while":   whileOp,
		"trim":    trimOp,
		"lower":   lowerOp,
		"upper":   upperOp,
		"to_string": toStringOp,
		"concat": concatOp,
		"break": breakOp,
		"continue": continueOp,
		"len":      lenOp,
		"keys": keysOp, 
		"values": valuesOp,
		"noop": noopOp,
	}
}

func noopOp(jp *JispProgram, _ *JispOperation) error {
	// No operation, do nothing.
	return nil
}

// ExecuteOperations iterates and executes a slice of JispOperations.
func (jp *JispProgram) ExecuteOperations(ops []JispOperation) error {
	for _, op := range ops {
		handler, found := operations[op.Name]
		if !found {
			return fmt.Errorf("unknown operation: %s", op.Name)
		}
		if err := handler(jp, &op); err != nil { // Pass the whole op struct
			// Propagate break and continue signals without wrapping, and stop execution of current operations list
			if _, isBreak := err.(*BreakSignal); isBreak {
				return err
			}
			if _, isContinue := err.(*ContinueSignal); isContinue {
				return err
			}
			return fmt.Errorf("error executing operation '%s': %w", op.Name, err)
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


func trimOp(jp *JispProgram, _ *JispOperation) error {
	val, err := jp.popString("trim")
	if err != nil {
		return err
	}
	jp.Push(strings.TrimSpace(val))
	return nil
}

func lowerOp(jp *JispProgram, _ *JispOperation) error {
	val, err := jp.popString("lower")
	if err != nil {
		return err
	}
	jp.Push(strings.ToLower(val))
	return nil
}

func upperOp(jp *JispProgram, _ *JispOperation) error {
	val, err := jp.popString("upper")
	if err != nil {
		return err
	}
	jp.Push(strings.ToUpper(val))
	return nil
}

func toStringOp(jp *JispProgram, _ *JispOperation) error {
	val, err := jp.popValue("to_string")
	if err != nil {
		return err
	}
	jp.Push(fmt.Sprintf("%v", val))
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
	jp.Variables[key] = val
	return nil
}

// Get retrieves a value from the Variables map and pushes it onto the stack.
func (jp *JispProgram) Get() error {
	key, err := jp.popString("get")
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
	key, err := jp.popString("exists")
	if err != nil {
		return err
	}
	_, found := jp.Variables[key]
	jp.Push(found)
	return nil
}

// Delete removes a variable from the Variables map.
func (jp *JispProgram) Delete() error {
	key, err := jp.popString("delete")
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
	a, b, err := jp.popTwoValues("eq")
	if err != nil {
		return err
	}
	jp.Push(a == b)
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
	val, err := jp.popValue("not")
	if err != nil {
		return err
	}
	boolVal, ok := val.(bool)
	if !ok {
		return fmt.Errorf("not error: expected a boolean on stack, got %T", val)
	}
	jp.Push(!boolVal)
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

// --- Helper Functions ---

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

func (jp *JispProgram) popString(opName string) (string, error) {
	val, err := jp.popValue(opName)
	if err != nil {
		return "", err
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%s error: expected a string key on stack, got %T", opName, val)
	}
	return str, nil
}

func (jp *JispProgram) popKeyValue(opName string) (string, interface{}, error) {
	if len(jp.Stack) < 2 {
		return "", nil, fmt.Errorf("stack underflow for %s: expected value and key on stack", opName)
	}
	keyVal, ok := jp.Stack[len(jp.Stack)-1].(string)
	if !ok {
		return "", nil, fmt.Errorf("%s error: expected a string key on top of stack, got %T", jp.Stack[len(jp.Stack)-1], opName)
	}
	value := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2]
	return keyVal, value, nil
}

func (jp *JispProgram) applyNumericBinaryOp(opName string, op func(float64, float64) (interface{}, error)) error {
	a, b, err := jp.popTwoValues(opName)
	if err != nil {
		return err
	}
	numA, okA := a.(float64)
	numB, okB := b.(float64)
	if !okA || !okB {
		return fmt.Errorf("%s error: expected two numbers, got %T and %T", opName, a, b)
	}

	res, err := op(numA, numB)
	if err != nil {
		return fmt.Errorf("%s error: %w", opName, err)
	}
	jp.Push(res)
	return nil
}

func (jp *JispProgram) applyBooleanBinaryOp(opName string, op func(bool, bool) bool) error {
	a, b, err := jp.popTwoValues(opName)
	if err != nil {
		return err
	}
	boolA, okA := a.(bool)
	boolB, okB := b.(bool)
	if !okA || !okB {
		return fmt.Errorf("%s error: expected two booleans, got %T and %T", opName, a, b)
	}
	jp.Push(op(boolA, boolB))
	return nil
}

func (jp *JispProgram) applyComparisonOp(opName string, opNum func(float64, float64) bool, opStr func(string, string) bool) error {
	a, b, err := jp.popTwoValues(opName)
	if err != nil {
		return err
	}
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
	for i, v := range bodyArr {
		jsonData, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal raw operation: %w", err)
		}
		var op JispOperation
		if err := json.Unmarshal(jsonData, &op); err != nil {
			return nil, fmt.Errorf("failed to unmarshal operation from raw data: %w", err)
		}
		ops[i] = op
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

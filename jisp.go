package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// JispProgram represents the entire state of a JISP program.
type JispProgram struct {
	Stack     []interface{}          `json:"stack"`
	Variables map[string]interface{} `json:"variables"`
	State     map[string]interface{} `json:"state"` // For pop operation target
}

// JispOperation represents a single operation in a JISP program.
type JispOperation struct {
	Op   string      `json:"op"`
	Args interface{} `json:"args"`
}

// JispCode represents the code part of a JISP program.
type JispCode struct {
	Code []JispOperation `json:"code"`
}

// operationHandler defines the signature for all JISP operations.
type operationHandler func(jp *JispProgram, args interface{}) error

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
		"and":    andOp,
		"or":     orOp,
		"not":    notOp,
		"if":     ifOp,
	}
}

// ExecuteOperations iterates and executes a slice of JispOperations.
func (jp *JispProgram) ExecuteOperations(ops []JispOperation) error {
	for _, op := range ops {
		handler, found := operations[op.Op]
		if !found {
			return fmt.Errorf("unknown operation: %s", op.Op)
		}
		if err := handler(jp, op.Args); err != nil {
			return fmt.Errorf("error executing operation '%s': %w", op.Op, err)
		}
	}
	return nil
}

// --- Operation Handlers ---

func pushOp(jp *JispProgram, args interface{}) error {
	jp.Push(args)
	return nil
}

func popOp(jp *JispProgram, args interface{}) error {
	fieldName, ok := args.(string)
	if !ok {
		return fmt.Errorf("pop error: expected string argument for fieldName, got %T", args)
	}
	return jp.Pop(fieldName)
}

func setOp(jp *JispProgram, _ interface{}) error    { return jp.Set() }
func getOp(jp *JispProgram, _ interface{}) error    { return jp.Get() }
func existsOp(jp *JispProgram, _ interface{}) error { return jp.Exists() }
func deleteOp(jp *JispProgram, _ interface{}) error { return jp.Delete() }
func eqOp(jp *JispProgram, _ interface{}) error     { return jp.Eq() }
func ltOp(jp *JispProgram, _ interface{}) error     { return jp.Lt() }
func gtOp(jp *JispProgram, _ interface{}) error     { return jp.Gt() }
func addOp(jp *JispProgram, _ interface{}) error    { return jp.Add() }
func subOp(jp *JispProgram, _ interface{}) error    { return jp.Sub() }
func mulOp(jp *JispProgram, _ interface{}) error    { return jp.Mul() }
func divOp(jp *JispProgram, _ interface{}) error    { return jp.Div() }
func andOp(jp *JispProgram, _ interface{}) error    { return jp.And() }
func orOp(jp *JispProgram, _ interface{}) error     { return jp.Or() }
func notOp(jp *JispProgram, _ interface{}) error    { return jp.Not() }

func ifOp(jp *JispProgram, args interface{}) error {
	argsArray, ok := args.([]interface{})
	if !ok || len(argsArray) < 1 || len(argsArray) > 2 {
		return fmt.Errorf("if error: expected 1 or 2 array arguments for then/else bodies, got %v", args)
	}

	thenBody, err := parseJispOps(argsArray[0])
	if err != nil {
		return fmt.Errorf("if error in 'then' body: %w", err)
	}

	var elseBody []JispOperation
	if len(argsArray) == 2 {
		elseBody, err = parseJispOps(argsArray[1])
		if err != nil {
			return fmt.Errorf("if error in 'else' body: %w", err)
		}
	}
	return jp.If(thenBody, elseBody)
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
		opMap, isMap := v.(map[string]interface{})
		if !isMap {
			return nil, fmt.Errorf("expected operation to be an object, got %T", v)
		}
		op, ok := opMap["op"].(string)
		if !ok {
			return nil, fmt.Errorf("operation 'op' must be a string")
		}
		ops[i] = JispOperation{Op: op, Args: opMap["args"]}
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

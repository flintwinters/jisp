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

// Push adds a value to the top of the stack.
func (jp *JispProgram) Push(value interface{}) {
	jp.Stack = append(jp.Stack, value)
}

// Pop removes the top value from the stack and moves it to the program state field specified by fieldName.
func (jp *JispProgram) Pop(fieldName string) error {
	if len(jp.Stack) == 0 {
		return fmt.Errorf("stack underflow: cannot pop from an empty stack")
	}
	value := jp.Stack[len(jp.Stack)-1]
	jp.Stack = jp.Stack[:len(jp.Stack)-1] // Remove value

	if jp.State == nil {
		jp.State = make(map[string]interface{})
	}
	jp.State[fieldName] = value
	return nil
}

// Set stores a value from the stack into the Variables map using a key from the stack.
func (jp *JispProgram) Set() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for set: expected value and key on stack")
	}

	keyVal := jp.Stack[len(jp.Stack)-1]
	key, ok := keyVal.(string)
	if !ok {
		return fmt.Errorf("set error: expected a string key on top of stack, got %T", keyVal)
	}
	jp.Stack = jp.Stack[:len(jp.Stack)-1] // Remove key

	value := jp.Stack[len(jp.Stack)-1]
	jp.Stack = jp.Stack[:len(jp.Stack)-1] // Remove value

	if jp.Variables == nil {
		jp.Variables = make(map[string]interface{})
	}
	jp.Variables[key] = value
	return nil
}

// Get retrieves a value from the Variables map using a key from the stack and pushes it onto the stack.
func (jp *JispProgram) Get() error {
	if len(jp.Stack) == 0 {
		return fmt.Errorf("stack underflow for get: expected key on stack")
	}

	keyVal := jp.Stack[len(jp.Stack)-1]
	key, ok := keyVal.(string)
	if !ok {
		return fmt.Errorf("get error: expected a string key on top of stack, got %T", keyVal)
	}
	jp.Stack = jp.Stack[:len(jp.Stack)-1] // Remove key

	val, found := jp.Variables[key]
	if !found {
		return fmt.Errorf("get error: variable '%s' not found", key)
	}
	jp.Push(val)
	return nil
}

// Exists checks if a variable exists in the Variables map and pushes the boolean result onto the stack.
func (jp *JispProgram) Exists() error {
	if len(jp.Stack) == 0 {
		return fmt.Errorf("stack underflow for exists: expected key on stack")
	}

	keyVal := jp.Stack[len(jp.Stack)-1]
	key, ok := keyVal.(string)
	if !ok {
		return fmt.Errorf("exists error: expected a string key on top of stack, got %T", keyVal)
	}
	jp.Stack = jp.Stack[:len(jp.Stack)-1] // Remove key

	_, found := jp.Variables[key]
	jp.Push(found)
	return nil
}

// Delete removes a variable from the Variables map.
func (jp *JispProgram) Delete() error {
	if len(jp.Stack) == 0 {
		return fmt.Errorf("stack underflow for delete: expected key on stack")
	}

	keyVal := jp.Stack[len(jp.Stack)-1]
	key, ok := keyVal.(string)
	if !ok {
		return fmt.Errorf("delete error: expected a string key on top of stack, got %T", keyVal)
	}
	jp.Stack = jp.Stack[:len(jp.Stack)-1] // Remove key

	if jp.Variables != nil {
		delete(jp.Variables, key)
	}
	return nil
}

// Eq pops two values, checks for strict equality, and pushes the boolean result onto the stack.
func (jp *JispProgram) Eq() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for eq: expected two values on stack")
	}

	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b

	jp.Push(a == b)
	return nil
}

// Lt pops two values, checks if the first is less than the second, and pushes the boolean result onto the stack.
func (jp *JispProgram) Lt() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for lt: expected two values on stack")
	}

	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b

	switch vA := a.(type) {
	case float64:
		vB, ok := b.(float64)
		if !ok {
			return fmt.Errorf("lt error: cannot compare number with %T", b)
		}
		jp.Push(vA < vB)
	case string:
		vB, ok := b.(string)
		if !ok {
			return fmt.Errorf("lt error: cannot compare string with %T", b)
		}
		jp.Push(vA < vB)
	default:
		return fmt.Errorf("lt error: unsupported type for comparison: %T", a)
	}
	return nil
}

// Gt pops two values, checks if the first is greater than the second, and pushes the boolean result onto the stack.
func (jp *JispProgram) Gt() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for gt: expected two values on stack")
	}

	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b

	switch vA := a.(type) {
	case float64:
		vB, ok := b.(float64)
		if !ok {
			return fmt.Errorf("gt error: cannot compare number with %T", b)
		}
		jp.Push(vA > vB)
	case string:
		vB, ok := b.(string)
		if !ok {
			return fmt.Errorf("gt error: cannot compare string with %T", b)
		}
		jp.Push(vA > vB)
	default:
		return fmt.Errorf("gt error: unsupported type for comparison: %T", a)
	}
	return nil
}

// Add pops two numbers, adds them, and pushes the result onto the stack.
func (jp *JispProgram) Add() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for add: expected two numbers on stack")
	}

	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b

	numA, okA := a.(float64)
	numB, okB := b.(float64)
	if !okA || !okB {
		return fmt.Errorf("add error: expected two numbers on stack, got %T and %T", a, b)
	}
	jp.Push(numA + numB)
	return nil
}

// Sub pops two numbers, subtracts the second from the first, and pushes the result onto the stack.
func (jp *JispProgram) Sub() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for sub: expected two numbers on stack")
	}

	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b

	numA, okA := a.(float64)
	numB, okB := b.(float64)
	if !okA || !okB {
		return fmt.Errorf("sub error: expected two numbers on stack, got %T and %T", a, b)
	}
	jp.Push(numA - numB)
	return nil
}

// Mul pops two numbers, multiplies them, and pushes the result onto the stack.
func (jp *JispProgram) Mul() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for mul: expected two numbers on stack")
	}

	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b

	numA, okA := a.(float64)
	numB, okB := b.(float64)
	if !okA || !okB {
		return fmt.Errorf("mul error: expected two numbers on stack, got %T and %T", a, b)
	}
	jp.Push(numA * numB)
	return nil
}

// Div pops two numbers, divides the first by the second, and pushes the result onto the stack.
func (jp *JispProgram) Div() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for div: expected two numbers on stack")
	}

	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b

	numA, okA := a.(float64)
	numB, okB := b.(float64)
	if !okA || !okB {
		return fmt.Errorf("div error: expected two numbers on stack, got %T and %T", a, b)
	}
	if numB == 0 {
		return fmt.Errorf("div error: division by zero")
	}
	jp.Push(numA / numB)
	return nil
}

// And pops two booleans, performs logical AND, and pushes the result onto the stack.
func (jp *JispProgram) And() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for and: expected two booleans on stack")
	}

	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b

	boolA, okA := a.(bool)
	boolB, okB := b.(bool)
	if !okA || !okB {
		return fmt.Errorf("and error: expected two booleans on stack, got %T and %T", a, b)
	}
	jp.Push(boolA && boolB)
	return nil
}

// Or pops two booleans, performs logical OR, and pushes the result onto the stack.
func (jp *JispProgram) Or() error {
	if len(jp.Stack) < 2 {
		return fmt.Errorf("stack underflow for or: expected two booleans on stack")
	}

	b := jp.Stack[len(jp.Stack)-1]
	a := jp.Stack[len(jp.Stack)-2]
	jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b

	boolA, okA := a.(bool)
	boolB, okB := b.(bool)
	if !okA || !okB {
		return fmt.Errorf("or error: expected two booleans on stack, got %T and %T", a, b)
	}
	jp.Push(boolA || boolB)
	return nil
}

// Not pops a boolean, performs logical NOT, and pushes the result onto the stack.
func (jp *JispProgram) Not() error {
	if len(jp.Stack) < 1 {
		return fmt.Errorf("stack underflow for not: expected a boolean on stack")
	}

	val := jp.Stack[len(jp.Stack)-1]
	jp.Stack = jp.Stack[:len(jp.Stack)-1] // Pop value

	boolVal, ok := val.(bool)
	if !ok {
		return fmt.Errorf("not error: expected a boolean on stack, got %T", val)
	}
	jp.Push(!boolVal)
	return nil
}

// If conditionally executes operations based on a boolean popped from the stack.
func (jp *JispProgram) If(thenBody, elseBody []JispOperation) error {
	if len(jp.Stack) == 0 {
		return fmt.Errorf("stack underflow for if: expected boolean condition on stack")
	}

	conditionVal := jp.Stack[len(jp.Stack)-1]
	jp.Stack = jp.Stack[:len(jp.Stack)-1] // Remove condition

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

// JispOperation represents a single operation in a JISP program.
type JispOperation struct {
	Op   string      `json:"op"`
	Args interface{} `json:"args"`
}

// JispCode represents the code part of a JISP program.
type JispCode struct {
	Code []JispOperation `json:"code"`
}

// ExecuteOperations iterates and executes a slice of JispOperations.
func (jp *JispProgram) ExecuteOperations(ops []JispOperation) error {
	for _, op := range ops {
		var err error
		switch op.Op {
		case "push":
			jp.Push(op.Args)
		case "pop":
			fieldName, ok := op.Args.(string)
			if !ok {
				err = fmt.Errorf("pop error: expected string argument for fieldName, got %T", op.Args)
			} else {
				err = jp.Pop(fieldName)
			}
		case "set":
			err = jp.Set()
		case "get":
			err = jp.Get()
		case "exists":
			err = jp.Exists()
		case "delete":
			err = jp.Delete()
		case "eq":
			err = jp.Eq()
		case "lt":
			err = jp.Lt()
		case "gt":
			err = jp.Gt()
		case "add":
			err = jp.Add()
		case "sub":
			err = jp.Sub()
		case "mul":
			err = jp.Mul()
		case "div":
			err = jp.Div()
		case "and":
			err = jp.And()
		case "or":
			err = jp.Or()
		case "not":
			err = jp.Not()
		case "if":
			argsArray, ok := op.Args.([]interface{})
			if !ok || len(argsArray) < 1 || len(argsArray) > 2 {
				return fmt.Errorf("if error: expected 1 or 2 array arguments for then/else bodies, got %v", op.Args)
			}

			thenBodyRaw, ok := argsArray[0].([]interface{})
			if !ok {
				return fmt.Errorf("if error: expected 'then' body to be an array of operations, got %T", argsArray[0])
			}
			thenBody := make([]JispOperation, len(thenBodyRaw))
			for i, v := range thenBodyRaw {
				opMap, isMap := v.(map[string]interface{})
				if !isMap {
					return fmt.Errorf("if error: expected 'then' body operation to be an object, got %T", v)
				}
				thenBody[i] = JispOperation{
					Op:   opMap["op"].(string),
					Args: opMap["args"],
				}
			}

			var elseBody []JispOperation
			if len(argsArray) == 2 {
				elseBodyRaw, ok := argsArray[1].([]interface{})
				if !ok {
					return fmt.Errorf("if error: expected 'else' body to be an array of operations, got %T", argsArray[1])
				}
				elseBody = make([]JispOperation, len(elseBodyRaw))
				for i, v := range elseBodyRaw {
					opMap, isMap := v.(map[string]interface{})
					if !isMap {
						return fmt.Errorf("if error: expected 'else' body operation to be an object, got %T", v)
					}
					elseBody[i] = JispOperation{
						Op:   opMap["op"].(string),
						Args: opMap["args"],
					}
				}
			}
			err = jp.If(thenBody, elseBody)

		default:
			err = fmt.Errorf("unknown operation: %s", op.Op)
		}

		if err != nil {
			return fmt.Errorf("Error executing operation '%s': %v", op.Op, err)
		}
	}
	return nil
}

func main() {
	jp := &JispProgram{
		Stack:     []interface{}{},
		Variables: make(map[string]interface{}),
		State:     make(map[string]interface{}),
	}

	// Read JISP program from stdin
	var jispCode JispCode
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&jispCode); err != nil {
		log.Fatalf("Error reading JISP program from stdin: %v", err)
	}

	// Execute JISP operations
	if err := jp.ExecuteOperations(jispCode.Code); err != nil {
		log.Fatalf("Error during program execution: %v", err)
	}

	// Output final JispProgram state as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jp); err != nil {
		log.Fatalf("Error encoding JISP program state to stdout: %v", err)
	}
	os.Exit(0)
}

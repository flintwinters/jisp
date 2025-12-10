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

// JispOperation represents a single operation in a JISP program.
type JispOperation struct {
	Op   string      `json:"op"`
	Args interface{} `json:"args"`
}

// JispCode represents the code part of a JISP program.
type JispCode struct {
	Code []JispOperation `json:"code"`
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
	for _, op := range jispCode.Code {
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
		case "add":
			if len(jp.Stack) < 2 {
				err = fmt.Errorf("stack underflow for add: expected two numbers on stack")
			} else {
				a := jp.Stack[len(jp.Stack)-2]
				b := jp.Stack[len(jp.Stack)-1]
				jp.Stack = jp.Stack[:len(jp.Stack)-2] // Pop a and b
				numA, okA := a.(float64) // JSON numbers are float64
				numB, okB := b.(float64)
				if !okA || !okB {
					err = fmt.Errorf("add error: expected two numbers on stack, got %T and %T", a, b)
				} else {
					jp.Push(numA + numB)
				}
			}
		default:
			err = fmt.Errorf("unknown operation: %s", op.Op)
		}

		if err != nil {
			log.Fatalf("Error executing operation '%s': %v", op.Op, err)
		}
	}

	// Output final JispProgram state as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jp); err != nil {
		log.Fatalf("Error encoding JISP program state to stdout: %v", err)
	}
	os.Exit(0)
}

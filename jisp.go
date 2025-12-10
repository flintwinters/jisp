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

func main() {
	fmt.Println("Hello from JISP Go!")

	// Simple JSON parsing sanity check (kept for now, but not directly related to JISP ops)
	jsonString := `{"name": "Jisp Core", "version": "0.1.0"}`
	var config struct{ Name, Version string }
	err := json.Unmarshal([]byte(jsonString), &config)
	if err != nil {
		log.Printf("Warning: Error parsing JSON in main (JispConfig): %v\n", err)
	} else {
		fmt.Printf("Parsed Jisp Config: Name=%s, Version=%s\n", config.Name, config.Version)
	}

	// Run all JISP operation tests
	if !TestPush() || !TestSetGetPop() {
		fmt.Println("One or more JISP operation tests failed!")
		os.Exit(1) // Indicate failure
	}
	os.Exit(0) // Indicate success
}

// TestPush performs a simple test of the Push operation.
func TestPush() bool {
	jp := &JispProgram{
		Stack:     []interface{}{},
		Variables: make(map[string]interface{}),
		State:     make(map[string]interface{}),
	}

	jp.Push(10)
	jp.Push("hello")
	jp.Push(true)

	expectedStack := []interface{}{10, "hello", true}

	if len(jp.Stack) != len(expectedStack) {
		fmt.Printf("TestPush: Stack length mismatch. Expected %d, got %d\n", len(expectedStack), len(jp.Stack))
		return false
	}

	for i, v := range jp.Stack {
		if v != expectedStack[i] {
			fmt.Printf("TestPush: Stack content mismatch at index %d. Expected %v, got %v\n", i, expectedStack[i], v)
			return false
		}
	}

	fmt.Println("TestPush: Push operation check passed.")
	return true
}

// TestSetGetPop performs a combined test of Set, Get, and Pop operations.
func TestSetGetPop() bool {
	// Test Case 1: Set operation
	jp1 := &JispProgram{
		Stack:     []interface{}{},
		Variables: make(map[string]interface{}),
		State:     make(map[string]interface{}),
	}
	jp1.Push("test_value_set")
	jp1.Push("test_key_set")
	if err := jp1.Set(); err != nil {
		fmt.Printf("TestSetGetPop: Set operation failed: %v\n", err)
		return false
	}
	if val, found := jp1.Variables["test_key_set"]; !found || val != "test_value_set" {
		fmt.Printf("TestSetGetPop: Set operation failed, variable not set correctly. Got %v\n", val)
		return false
	}

	// Test Case 2: Get operation
	jp2 := &JispProgram{
		Stack:     []interface{}{},
		Variables: map[string]interface{}{"test_key_get": "test_value_get"},
		State:     make(map[string]interface{}),
	}
	jp2.Push("test_key_get")
	if err := jp2.Get(); err != nil {
		fmt.Printf("TestSetGetPop: Get operation failed: %v\n", err)
		return false
	}
	if len(jp2.Stack) != 1 || jp2.Stack[0] != "test_value_get" {
		fmt.Printf("TestSetGetPop: Get operation failed, stack content mismatch. Expected [test_value_get], got %v\n", jp2.Stack)
		return false
	}

	// Test Case 3: Pop operation
	jp3 := &JispProgram{
		Stack:     []interface{}{},
		Variables: make(map[string]interface{}),
		State:     make(map[string]interface{}),
	}
	jp3.Push(42)
	if err := jp3.Pop("result_field"); err != nil {
		fmt.Printf("TestSetGetPop: Pop operation failed: %v\n", err)
		return false
	}
	if len(jp3.Stack) != 0 {
		fmt.Printf("TestSetGetPop: Pop operation failed, stack not empty. Got %v\n", jp3.Stack)
		return false
	}
	if val, found := jp3.State["result_field"]; !found || val != 42 {
		fmt.Printf("TestSetGetPop: Pop operation failed, state field not set correctly. Expected 42, got %v\n", val)
		return false
	}

	fmt.Println("TestSetGetPop: Set, Get, and Pop operations check passed.")
	return true
}
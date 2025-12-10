package main

import (
	"encoding/json"
	"fmt"
	"log"
)

// JispConfig represents a simple configuration structure for JISP.
type JispConfig struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func main() {
	fmt.Println("Hello from JISP Go!")

	// Simple JSON parsing sanity check
	jsonString := `{"name": "Jisp Core", "version": "0.1.0"}`
	var config JispConfig
	err := json.Unmarshal([]byte(jsonString), &config)
	if err != nil {
		log.Fatalf("Error parsing JSON in main: %v", err)
	}

	fmt.Printf("Parsed Jisp Config: Name=%s, Version=%s\n", config.Name, config.Version)

	if !TestJisp() {
		log.Fatal("TestJisp failed!")
	}
}

// TestJisp performs a simple test of the JSON parsing.
func TestJisp() bool {
	jsonString := `{"name": "Test Config", "version": "1.0.0"}`
	var config JispConfig
	err := json.Unmarshal([]byte(jsonString), &config)
	if err != nil {
		fmt.Printf("TestJisp: Error parsing JSON: %v\n", err)
		return false
	}

	if config.Name != "Test Config" || config.Version != "1.0.0" {
		fmt.Printf("TestJisp: JSON parsing mismatch. Expected Name='Test Config', Version='1.0.0', got Name='%s', Version='%s'\n", config.Name, config.Version)
		return false
	}

	fmt.Println("TestJisp: JSON parsing check passed.")
	return true
}
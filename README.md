
<div align="center">
<img src="https://qforj.com/jisp.png" height="64px">

# JISP

[https://jisp.world](https://jisp.world)

![Discord](https://kindalign.com/discord)
![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)
![GitHub Repo stars](https://img.shields.io/github/stars/flintwinters/jisp)

</div>

Jisp is a programming _system_ which uses the **JSON data model** as its "atomic fabric".  You can write Jisp code in any JSON-compatible data format - [ION](https://amazon-ion.github.io/ion-docs/), TOML, YAML, TOON, JSON, etc.

The core philosophy is to maintain all program state (including code, variables, and execution flow) in a single self-contained JSON object. This design ensures that every aspect of a **running** Jisp program can be effortlessly exported, imported, saved, loaded, diffed, restored, etc. at ***any point*** during execution.

This approach makes it easier to debug, integrate with other tools, and the strict, simple, universally understandable grammar is perfect for LLM outputs.

### Why JSON?


#### 1. Advanced Debugging

Jisp offers powerful debugging features thanks to its use of JSON for the entire program state—code, heap, stack, and environment. It is imperative that all program state be captured by JSON to maintain this behavior. By encapsulating everything as a single JSON object, you can:

* **Step Forward and Backward**: Log the program state at different points and jump between them.

* **Time Travel with Diffs**: Track program changes using simple diffs, allowing you to revert to previous execution states or apply inverse diffs.

* **Full Inspection**: Inspect and manipulate any part of the program (variables, functions, memory) at any time, all through the transparent structure of JSON.

* ***Automated* Debugging**: Scripts and LLMs can programmatically inspect program states, enabling automated use of the debugger.

#### 2. Easy Integration:
Jisp’s use of JSON makes it easy to work with other systems and tools, such as APIs or language models. JSON is a universal format for data exchange, so connecting Jisp with external tools is seamless.

#### 3. Simple, Readable Code:
Jisp programs are written in JSON, a format most developers are already familiar with. This makes the code easy to read and understand without dealing with new syntax.

### Conclusion

Jisp takes advantage of JSON's simplicity and universality to create a programming system that’s easy to understand, easy to debug, and easy to integrate.

### Installation

You can download the pre-compiled Go binary from the [releases page](https://github.com/flintwinters/jisp/releases/tag/release).

### Build

To build Jisp from source, clone the repository and run the `build.py` script:

```bash
git clone https://github.com/flintwinters/jisp.git
cd jisp
python3 build.py
```

### Test

Run all checks:

`python3 build.py`

### Usage

To run a Jisp program, simply execute the `jisp` binary with your JSON-formatted Jisp code:

```bash
jisp example.json
```

### Examples

These are directly taken from the demos at [https://jisp.world](https://jisp.world)

#### Json Schema Validation

Demonstrates schema validation using the 'valid' operation. It defines a schema for a user object in the 'variables' section and then tests both a valid and an invalid object against it. The boolean results of the validation are stored in variables.

You can of course use this to validate Jisp code structure itself.

```json
{
    "variables": {
        "user_schema": {
            "type": "object",
            "properties": {
                "username": {"type": "string", "minLength": 3},
                "email": {"type": "string", "format": "email"},
                "age": {"type": "number", "minimum": 18}
            },
            "required": ["username", "email", "age"]
        }
    },
    "code": [
    ["get", "user_schema"],
    ["push", {"username": "jdoe", "email": "jdoe@example.com", "age": 30}],
    ["valid"],
    ["pop", "valid_user_check"],
    
    ["get", "user_schema"],
    ["push", {"username": "jsmith", "email": "not-an-email", "age": 17}],
    ["valid"],
    ["pop", "invalid_user_check"]
    ]
}
```

#### Reversible debugging:

Demonstrates procedural and reversible debugging. A sub-program is executed with a breakpoint, which causes an instruction to be skipped. The program is then advanced with 'step' and reverted with 'undo' to showcase time-travel debugging. The final state is captured for inspection.

```json
{
    "code": [
    ["push", {
        "save_history": true,
        "debug": true,
        "breakpoints": [["code", 1]],
        "code": [
            ["push", 5.0],
            ["pop", "a"],
            ["push", 10.0],
            ["pop", "b"]
        ]
    }],
    ["run"],
    ["step"],
    ["step"],
    ["undo"],
    ["pop", "final_debug_state"]
    ]
}
```


<div align="center">
<img src="https://qforj.com/jisp.png" height="64px">

# JISP

![Discord](https://img.shields.io/discord/1392579835012055131?logo=discord&logoColor=white&color=red)
![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)
![GitHub Repo stars](https://img.shields.io/github/stars/flintwinters/jisp)
<!-- ![Web Demo](https://img.shields.io/website?url=https%3A%2F%2Fqforj.com) -->
<!-- ![Read the Docs](https://img.shields.io/readthedocs/:jisp) -->


</div>

Jisp is a stack-based programming system which uses the JSON data model as its underlying atomic fabric. All arguments for operations are implicitly taken from the stack. The program, including the code, variables, and execution state, are all represented directly in a single JSON object. This approach makes it easier to debug, integrate with other tools, and eliminates LLM syntax errors.

The strict, simple, universally understandable grammar is perfect for LLM outputs and toolcalls, eliminating syntax errors.

### Why JSON?

The core philosophy of Jisp is to maintain all program state (including the stack, variables, and execution flow) entirely within a single, self-contained JSON object. This design ensures that every aspect of a running Jisp program can be effortlessly exported, imported, saved, and restored at any point during execution, providing unparalleled transparency and control over the program.

#### 1. Advanced Debugging

Jisp offers powerful debugging features thanks to its use of JSON for the entire program state—code, heap, stack, and environment. Since everything is encapsulated as a single JSON object, you can:

* **Step Forward and Backward**: Log the program state at different points and jump between them, replaying specific steps in the execution flow.

* **Time Travel with Diffs**: Track program changes using simple diffs, allowing you to easily revert to previous states or inspect a specific moment by applying inverse diffs.

* **Full Inspection**: Inspect and manipulate any part of the program (variables, functions, memory) at any time, all through the transparent structure of JSON.

* **Automated Debugging**: Scripts and LLMs can programmatically inspect program states, enabling automated debugging.

#### 2. Easy Integration:
Jisp’s use of JSON makes it easy to work with other systems and tools, such as APIs or language models. JSON is a common format for data exchange, so connecting Jisp with external tools is seamless.

#### 3. Simple, Readable Code:
Jisp programs are written in JSON, a format most developers are already familiar with. This makes the code easy to read and understand without dealing with new syntax.

### Conclusion

Jisp takes advantage of JSON's simplicity and universality to create a programming system that’s easy to understand, easy to debug, and easy to integrate.

### Build & Test

`python3 build.py`

### TODO

Make a decision on the ergonomics of map, filter, and reduce operations:

```json
["push", [1.0, 2.0, 3.0, 4.0]],
["push", "item"],
[
    "push", [
        ["push", "item"],
        ["get"],
        ["push", "item"],
        ["get"],
        ["mul"]
    ]
],
["map"]
```

Not happy with this explicit variable declaration.

#### Implementation TODOs:
- [ ] `call`
- [ ] `return`
- [x] `for`
- [x] `foreach`
- [x] `try`
- [x] `replace`
- [x] `len`
- [x] `slice`
- [x] `map`
- [x] `filter`
- [x] `reduce`
- [x] `sort`
- [x] `keys`
- [x] `values`
- [x] `union`
- [x] `intersection`
- [x] `difference`
- [ ] `join`
- [x] `range`
- [x] `noop`
- [x] `valid`
- [x] `raise`
- [x] `assert`


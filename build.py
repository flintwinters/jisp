import subprocess
import sys
import os
import json
import tempfile
import argparse
from rich.console import Console
from jsonschema import validate, ValidationError

console = Console()

GO_SOURCE_FILE = "jisp.go"
BINARY_NAME = "bin-jisp"
CHECKS_DIR = "checks"

# --- Error Message Constants ---
GO_COMPILATION_FAILED = "[bold red]❌ Go compilation failed.[/bold red]"
GO_COMMAND_NOT_FOUND = "[bold red]❌ Go command not found. Install Go or check PATH.[/bold red]"
CHECKS_DIR_NOT_FOUND = "[bold red]❌ Checks directory '{CHECKS_DIR}' not found.[/bold red]"
NO_CHECK_FILES_FOUND = "[bold yellow]⚠️ No check files found in '{CHECKS_DIR}'.[/bold yellow]"
ERROR_READING_CHECKS_DIR = "[bold red]❌ Error reading checks directory '{CHECKS_DIR}': {e}.[/bold red]"
JSON_DECODE_ERROR_IN_FILE = "  [bold red]❌ Error decoding JSON in {filepath}: {e}.[/bold red]"

# Test-specific failure messages (use .format() for dynamic parts)
TEST_FAILED_EXPECTED_ERROR = "Expected an error, but program succeeded."
TEST_FAILED_MSG_MISMATCH = "Error message mismatch in JSON output."
TEST_FAILED_EXPECTED_JSON = "Expected JSON output on stdout, but failed to decode."
TEST_FAILED_UNEXPECTED_EXEC_ERROR = "Unexpected error during test execution: {}"
TEST_FAILED_JISP_EXEC_ERROR = "JISP program execution error."
TEST_FAILED_INVALID_JISP_JSON = "Invalid JSON output from JISP binary: {}"
TEST_FAILED_VALIDATION_ERROR = "Validation error."

# Skipping messages
SKIPPING_TEST_MISSING_PROGRAM = "[bold yellow]⚠️ Skipping test '{description}' in {filepath}: Missing 'jisp_program'.[/bold yellow]"
SKIPPING_TEST_MISSING_SCHEMA_OR_ERROR = "[bold yellow]⚠️ Skipping test '{description}' in {filepath}: Missing 'validation_schema', 'expected_stack'/'expected_variables', or 'expected_error_message'.[/bold yellow]"

# Final summary messages
ALL_TESTS_PASSED = "[bold green]All {passed_tests} tests passed successfully![/bold green]"
TEST_SUMMARY = "[bold red]❌ {passed_tests}/{total_tests} tests passed.[/bold red]"
COMPILATION_FAILED = "[bold red]❌ Compilation failed.[/bold red]"
# --- End Error Message Constants ---

class _TestFailureException(Exception):
    """Custom exception to signal a test failure when fail-fast is enabled."""
    pass

def _handle_test_failure(fail_fast, description, checks_filepath, message, details=None):
    """Prints failure details and raises an exception if fail-fast is on."""
    _print_test_failure(description, checks_filepath, message)
    if details:
        for key, value in details.items():
            console.print(f"    {key}: {value}")
    if fail_fast:
        raise _TestFailureException()

def _print_test_failure(description: str, checks_filepath: str, message: str):
    """Helper to print a formatted test failure message."""
    console.print(f"  [bold red]❌ Test '{description}'\n[bold blue]{checks_filepath}[/bold blue] {message}[/bold red]")


def _merge_schemas(base, new):
    """Recursively merges the 'new' schema into the 'base' schema."""
    if not isinstance(base, dict) or not isinstance(new, dict):
        return new
    
    merged = base.copy()
    for key, new_val in new.items():
        if key in merged:
            merged[key] = _merge_schemas(merged[key], new_val)
        else:
            merged[key] = new_val
    return merged

def _combine_schemas(check):
    base_schema = check.get("validation_schema")
    # Make a deep copy to avoid side effects
    schema = json.loads(json.dumps(base_schema)) if base_schema else {}

    # Ensure basic structure
    schema.setdefault("type", "object")
    properties = schema.setdefault("properties", {})

    # Handle expected_stack (replaces any existing stack rule)
    if "expected_stack" in check:
        properties["stack"] = {"const": check["expected_stack"]}
    
    # Handle expected_variables (merges with existing variables rules)
    if "expected_variables" in check:
        variables_schema = properties.setdefault("variables", {})
        variables_schema.setdefault("type", "object")
        
        # Merge properties from expected_variables
        variable_props = variables_schema.setdefault("properties", {})
        for key, value in check["expected_variables"].items():
            variable_props[key] = {"const": value}
            
        # Merge required keys from expected_variables
        variable_req = variables_schema.setdefault("required", [])
        for key in check["expected_variables"]:
            if key not in variable_req:
                variable_req.append(key)

    # If no schema existed and no shorthands were provided, return None
    if not base_schema and "expected_stack" not in check and "expected_variables" not in check:
        return None
        
    # Apply defaults if sections are missing
    if "stack" not in properties:
        properties["stack"] = {"type": "array", "maxItems": 0}

    # Ensure top-level required fields
    required = schema.setdefault("required", [])
    if "stack" not in required: required.append("stack")
    if "variables" not in required: required.append("variables")

    return schema

def compile_go_program():
    if os.path.exists(BINARY_NAME) and os.path.getmtime(GO_SOURCE_FILE) < os.path.getmtime(BINARY_NAME):
        return True

    compile_command = ["go", "build", "-o", BINARY_NAME, GO_SOURCE_FILE]
    try:
        subprocess.run(compile_command, check=True, capture_output=True, text=True)
        return True
    except subprocess.CalledProcessError as e:
        console.print(GO_COMPILATION_FAILED)
        console.print(f"  Stderr: {e.stderr.strip()}")
        return False
    except FileNotFoundError:
        console.print(GO_COMMAND_NOT_FOUND)
        return False

def run_all_checks(fail_fast=False):
    total_tests = 0
    passed_tests = 0

    if not os.path.isdir(CHECKS_DIR):
        console.print(CHECKS_DIR_NOT_FOUND.format(CHECKS_DIR=CHECKS_DIR))
        return False

    try:
        check_files = sorted([f for f in os.listdir(CHECKS_DIR) if f.endswith('.json')])
    except OSError as e:
        console.print(ERROR_READING_CHECKS_DIR.format(CHECKS_DIR=CHECKS_DIR, e=e))
        return False

    if not check_files:
        console.print(NO_CHECK_FILES_FOUND.format(CHECKS_DIR=CHECKS_DIR))
        return True

    try:
        for checks_filename in check_files:
            checks_filepath = os.path.join(CHECKS_DIR, checks_filename)
            try:
                with open(checks_filepath, 'r') as f:
                    checks = json.load(f)
            except json.JSONDecodeError as e:
                console.print(JSON_DECODE_ERROR_IN_FILE.format(filepath=checks_filepath, e=e))
                if fail_fast:
                    return False
                continue

            for i, check in enumerate(checks if isinstance(checks, list) else [checks]):
                total_tests += 1
                description = check.get("description", f"Unnamed test {i+1}")
                jisp_program = check.get("jisp_program")
                validation_schema = _combine_schemas(check)
                expected_error_message = check.get("expected_error_message")

                if not jisp_program:
                    console.print(SKIPPING_TEST_MISSING_PROGRAM.format(description=description, filepath=checks_filepath))
                    continue

                if not validation_schema and not expected_error_message:
                    console.print(SKIPPING_TEST_MISSING_SCHEMA_OR_ERROR.format(description=description, filepath=checks_filepath))
                    continue

                temp_prog_filepath = None
                try:
                    with tempfile.NamedTemporaryFile(mode='w', delete=False, suffix=".json") as temp_f:
                        json.dump(jisp_program, temp_f)
                        temp_prog_filepath = temp_f.name

                    run_command = [f"./{BINARY_NAME}", temp_prog_filepath]

                    if expected_error_message:
                        try:
                            process = subprocess.run(run_command, capture_output=True, text=True)
                            if process.returncode == 0:
                                _handle_test_failure(fail_fast, description, checks_filepath, TEST_FAILED_EXPECTED_ERROR, {"Stdout": process.stdout.strip()})
                                continue

                            try:
                                output_json = json.loads(process.stdout)
                                error_details = output_json.get("error", {})
                                actual_message = error_details.get("message", "")
                                if expected_error_message in actual_message:
                                    passed_tests += 1
                                else:
                                    details = {
                                        "Expected to find": f"'{expected_error_message}'",
                                        "Actual message":   f"'{actual_message}'",
                                        "Full stdout": process.stdout.strip()
                                    }
                                    _handle_test_failure(fail_fast, description, checks_filepath, TEST_FAILED_MSG_MISMATCH, details)
                            except json.JSONDecodeError:
                                _handle_test_failure(fail_fast, description, checks_filepath, TEST_FAILED_EXPECTED_JSON, {"Stdout": process.stdout.strip()})
                        except Exception as e:
                            _handle_test_failure(fail_fast, description, checks_filepath, TEST_FAILED_UNEXPECTED_EXEC_ERROR.format(e))
                    else:
                        try:
                            process = subprocess.run(run_command, capture_output=True, text=True, check=True)
                            program_state = json.loads(process.stdout)
                            validate(instance=program_state, schema=validation_schema)
                            passed_tests += 1
                        except subprocess.CalledProcessError as e:
                            _handle_test_failure(fail_fast, description, checks_filepath, TEST_FAILED_JISP_EXEC_ERROR, {"Stderr": e.stderr.strip(), "Stdout": e.stdout.strip()})
                        except json.JSONDecodeError as e:
                            _handle_test_failure(fail_fast, description, checks_filepath, TEST_FAILED_INVALID_JISP_JSON.format(e), {"JISP Output": process.stdout.strip()})
                        except ValidationError as e:
                            details = {
                                "Error": e.message,
                                "Path": list(e.path),
                                "Expected": e.schema,
                                "Actual State": json.dumps(program_state, indent=2)
                            }
                            _handle_test_failure(fail_fast, description, checks_filepath, TEST_FAILED_VALIDATION_ERROR, details)
                        except Exception as e:
                            _handle_test_failure(fail_fast, description, checks_filepath, TEST_FAILED_UNEXPECTED_EXEC_ERROR.format(e))
                finally:
                    if temp_prog_filepath and os.path.exists(temp_prog_filepath):
                        os.remove(temp_prog_filepath)
    except _TestFailureException:
        return False

    if passed_tests == total_tests and total_tests > 0:
        console.print(ALL_TESTS_PASSED.format(passed_tests=passed_tests))
        return True
    console.print(TEST_SUMMARY.format(passed_tests=passed_tests, total_tests=total_tests))
    return False

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Build and test the JISP interpreter.")
    parser.add_argument("--fail-fast", action="store_false", help="Exit immediately when a single test fails.")
    args = parser.parse_args()

    if compile_go_program():
        if run_all_checks(fail_fast=args.fail_fast):
            sys.exit(0)
        sys.exit(1)
    console.print(COMPILATION_FAILED)
    sys.exit(1)
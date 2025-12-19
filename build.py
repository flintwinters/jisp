import subprocess
import sys
import os
import json
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
SKIPPING_TEST_MISSING_SCHEMA_OR_ERROR = "[bold yellow]⚠️ Skipping test '{description}' in {filepath}: Missing 'validation_schema' or 'expected_error_message'.[/bold yellow]"

# Final summary messages
ALL_TESTS_PASSED = "[bold green]All {passed_tests} tests passed successfully![/bold green]"
TEST_SUMMARY = "[bold red]❌ {passed_tests}/{total_tests} tests passed.[/bold red]"
COMPILATION_FAILED = "[bold red]❌ Compilation failed.[/bold red]"
# --- End Error Message Constants ---

def _print_test_failure(description: str, checks_filepath: str, message: str):
    """Helper to print a formatted test failure message."""
    console.print(f"  [bold red]❌ Test '{description}'\n[bold blue]{checks_filepath}[/bold blue] {message}[/bold red]")

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

def run_all_checks():
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

    for checks_filename in check_files:
        checks_filepath = os.path.join(CHECKS_DIR, checks_filename)
        try:
            with open(checks_filepath, 'r') as f:
                checks = json.load(f)
        except json.JSONDecodeError as e:
            console.print(JSON_DECODE_ERROR_IN_FILE.format(filepath=checks_filepath, e=e))
            continue

        for i, check in enumerate(checks):
            total_tests += 1
            description = check.get("description", f"Unnamed test {i+1}")
            jisp_program = check.get("jisp_program")
            validation_schema = check.get("validation_schema")
            expected_error_message = check.get("expected_error_message")

            if not jisp_program:
                console.print(SKIPPING_TEST_MISSING_PROGRAM.format(description=description, filepath=checks_filepath))
                continue

            if not validation_schema and not expected_error_message:
                console.print(SKIPPING_TEST_MISSING_SCHEMA_OR_ERROR.format(description=description, filepath=checks_filepath))
                continue

            if expected_error_message:
                try:
                    run_command = [f"./{BINARY_NAME}"]
                    process = subprocess.run(run_command, input=json.dumps(jisp_program), 
                                             capture_output=True, text=True)
                    
                    if process.returncode == 0:
                        _print_test_failure(description, checks_filepath, TEST_FAILED_EXPECTED_ERROR)
                        console.print(f"    Stdout: {process.stdout.strip()}")
                        continue

                    try:
                        output_json = json.loads(process.stdout)
                        error_details = output_json.get("error", {})
                        actual_message = error_details.get("message", "")
                        
                        if expected_error_message in actual_message:
                            passed_tests += 1
                        else:
                            _print_test_failure(description, checks_filepath, TEST_FAILED_MSG_MISMATCH)
                            console.print(f"    Expected to find: '{expected_error_message}'")
                            console.print(f"    Actual message:   '{actual_message}'")
                            console.print(f"    Full stdout: {process.stdout.strip()}")
                    except json.JSONDecodeError:
                        _print_test_failure(description, checks_filepath, TEST_FAILED_EXPECTED_JSON)
                        console.print(f"    Stdout: {process.stdout.strip()}")

                except Exception as e:
                    _print_test_failure(description, checks_filepath, TEST_FAILED_UNEXPECTED_EXEC_ERROR.format(e))
            else:
                try:
                    run_command = [f"./{BINARY_NAME}"]
                    process = subprocess.run(run_command, input=json.dumps(jisp_program), 
                                             capture_output=True, text=True, check=True)
                    
                    program_state = json.loads(process.stdout)
                    validate(instance=program_state, schema=validation_schema)
                    passed_tests += 1
                except subprocess.CalledProcessError as e:
                    _print_test_failure(description, checks_filepath, TEST_FAILED_JISP_EXEC_ERROR)
                    console.print(f"    Stderr: {e.stderr.strip()}")
                    console.print(f"    Stdout: {process.stdout.strip()}")
                except json.JSONDecodeError as e:
                    _print_test_failure(description, checks_filepath, TEST_FAILED_INVALID_JISP_JSON.format(e))
                    console.print(f"    JISP Output: {process.stdout.strip()}")
                except ValidationError as e:
                    _print_test_failure(description, checks_filepath, TEST_FAILED_VALIDATION_ERROR)
                    console.print(f"    Error: {e.message}")
                    console.print(f"    Path: {list(e.path)}")
                    console.print(f"    Expected: {e.schema}")
                    console.print(f"    Actual State: {json.dumps(program_state, indent=2)}")
                except Exception as e:
                    _print_test_failure(description, checks_filepath, TEST_FAILED_UNEXPECTED_EXEC_ERROR.format(e))
    
    if passed_tests == total_tests and total_tests > 0:
        console.print(ALL_TESTS_PASSED.format(passed_tests=passed_tests))
        return True
    console.print(TEST_SUMMARY.format(passed_tests=passed_tests, total_tests=total_tests))
    return False

if __name__ == "__main__":
    if compile_go_program():
        if run_all_checks():
            sys.exit(0)
        sys.exit(1)
    console.print(COMPILATION_FAILED)
    sys.exit(1)
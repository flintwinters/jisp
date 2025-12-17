import subprocess
import sys
import os
import json
from rich.console import Console
from jsonschema import validate, ValidationError

console = Console()

GO_SOURCE_FILE = "jisp.go"
BINARY_NAME = "bin-jisp"
CHECKS_FILE = "checks.json"

def compile_go_program():
    if os.path.exists(BINARY_NAME) and os.path.getmtime(GO_SOURCE_FILE) < os.path.getmtime(BINARY_NAME):
        return True

    compile_command = ["go", "build", "-o", BINARY_NAME, GO_SOURCE_FILE]
    try:
        subprocess.run(compile_command, check=True, capture_output=True, text=True)
        return True
    except subprocess.CalledProcessError as e:
        console.print("[bold red]❌ Go compilation failed.[/bold red]")
        console.print(f"  Stderr: {e.stderr.strip()}")
        return False
    except FileNotFoundError:
        console.print("[bold red]❌ Go command not found. Install Go or check PATH.[/bold red]")
        return False

def run_checks_from_json():
    total_tests = 0
    passed_tests = 0

    try:
        with open(CHECKS_FILE, 'r') as f:
            checks = json.load(f)
    except FileNotFoundError:
        console.print(f"[bold red]❌ {CHECKS_FILE} not found.[/bold red]")
        return False
    except json.JSONDecodeError as e:
        console.print(f"[bold red]❌ Error decoding {CHECKS_FILE}: {e}[/bold red]")
        return False

    for i, check in enumerate(checks):
        total_tests += 1
        description = check.get("description", f"Unnamed test {i+1}")
        jisp_program = check.get("jisp_program")
        validation_schema = check.get("validation_schema")
        expected_error_message = check.get("expected_error_message")

        if not jisp_program:
            console.print(f"[bold yellow]⚠️ Skipping test '{description}': Missing 'jisp_program'.[/bold yellow]")
            continue

        if not validation_schema and not expected_error_message:
            console.print(f"[bold yellow]⚠️ Skipping test '{description}': Missing 'validation_schema' or 'expected_error_message'.[/bold yellow]")
            continue

        if expected_error_message:
            # This test expects an error
            try:
                run_command = [f"./{BINARY_NAME}"]
                process = subprocess.run(run_command, input=json.dumps(jisp_program), 
                                         capture_output=True, text=True)
                
                if process.returncode == 0:
                    console.print(f"[bold red]❌ Test '{description}' FAILED: Expected an error, but program succeeded.[/bold red]")
                    console.print(f"  Stdout: {process.stdout.strip()}")
                    continue

                if expected_error_message in process.stderr:
                    passed_tests += 1
                else:
                    console.print(f"[bold red]❌ Test '{description}' FAILED: Error message mismatch.[/bold red]")
                    console.print(f"  Expected to find: '{expected_error_message}'")
                    console.print(f"  Actual Stderr:    '{process.stderr.strip()}'")

            except Exception as e:
                console.print(f"[bold red]❌ Test '{description}' FAILED: Unexpected error during test execution: {e}[/bold red]")
        else:
            # This test expects success
            try:
                run_command = [f"./{BINARY_NAME}"]
                process = subprocess.run(run_command, input=json.dumps(jisp_program), 
                                         capture_output=True, text=True, check=True)
                
                program_state = json.loads(process.stdout)
                validate(instance=program_state, schema=validation_schema)
                passed_tests += 1
            except subprocess.CalledProcessError as e:
                console.print(f"[bold red]❌ Test '{description}' FAILED: JISP program execution error.[/bold red]")
                console.print(f"  Stderr: {e.stderr.strip()}")
            except json.JSONDecodeError as e:
                console.print(f"[bold red]❌ Test '{description}' FAILED: Invalid JSON output from JISP binary: {e}[/bold red]")
                console.print(f"  JISP Output: {process.stdout.strip()}")
            except ValidationError as e:
                console.print(f"[bold red]❌ Test '{description}' FAILED: Validation error.[/bold red]")
                console.print(f"  Error: {e.message}")
                console.print(f"  Path: {list(e.path)}")
                console.print(f"  Expected: {e.schema}")
                console.print(f"  Actual State: {json.dumps(program_state, indent=2)}")
            except Exception as e:
                console.print(f"[bold red]❌ Test '{description}' FAILED: Unexpected error: {e}[/bold red]")
    
    if passed_tests == total_tests and total_tests > 0:
        console.print(f"[bold green]All {passed_tests} tests passed successfully![/bold green]")
        return True
    else:
        console.print(f"[bold red]{passed_tests}/{total_tests} tests passed.[/bold red]")
        return False

if __name__ == "__main__":
    if compile_go_program():
        if run_checks_from_json():
            sys.exit(0)
        else:
            console.print("[bold red]❌ JISP checks failed.[/bold red]")
            sys.exit(1)
    else:
        console.print("[bold red]❌ Compilation failed.[/bold red]")
        sys.exit(1)
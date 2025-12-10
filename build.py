import subprocess
import sys
import os
from rich.console import Console

console = Console()

GO_SOURCE_FILE = "jisp.go"
BINARY_NAME = "jisp-go-binary"

def compile_go_program():
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

def run_and_test_binary():
    run_command = [f"./{BINARY_NAME}"]
    try:
        result = subprocess.run(run_command, check=True, capture_output=True, text=True)
        console.print(f"[bold cyan]Binary Output[/bold cyan]\n{result.stdout.strip()}")

        if "TestJisp: JSON parsing check passed." in result.stdout:
            return True
        else:
            console.print("[bold red]❌ TestJisp did NOT report success.[/bold red]")
            return False

    except subprocess.CalledProcessError as e:
        console.print("[bold red]❌ Binary execution failed.[/bold red]")
        console.print(f"  Stderr: {e.stderr.strip()}")
        return False
    except FileNotFoundError:
        console.print(f"[bold red]❌ Binary '{BINARY_NAME}' not found. Compilation issue?[/bold red]")
        return False

def clean_up():
    if os.path.exists(BINARY_NAME):
        os.remove(BINARY_NAME)

if __name__ == "__main__":
    try:
        if compile_go_program():
            if run_and_test_binary():
                sys.exit(0)
            else:
                console.print("[bold red]❌ Tests failed.[/bold red]")
                sys.exit(1)
        else:
            console.print("[bold red]❌ Compilation failed.[/bold red]")
            sys.exit(1)
    finally:
        clean_up()

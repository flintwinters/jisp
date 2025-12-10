import subprocess
import sys
import os

GO_SOURCE_FILE = "jisp.go"
BINARY_NAME = "jisp-go-binary"

def compile_go_program():
    print(f"\033[1;34mCompiling Go program: {GO_SOURCE_FILE} ...\033[0m")
    compile_command = ["go", "build", "-o", BINARY_NAME, GO_SOURCE_FILE]
    try:
        result = subprocess.run(compile_command, check=True, capture_output=True, text=True)
        print("\033[1;32m✔ Go program compiled successfully.\033[0m")
        if result.stdout:
            print(f"Stdout: {result.stdout}")
        if result.stderr:
            print(f"Stderr: {result.stderr}")
        return True
    except subprocess.CalledProcessError as e:
        print("\033[1;31m❌ Go compilation failed.\033[0m")
        print(f"Stderr: {e.stderr}")
        print(f"Stdout: {e.stdout}")
        return False
    except FileNotFoundError:
        print("\033[1;31m❌ Go command not found. Please ensure Go is installed and in your PATH.\033[0m")
        return False

def run_and_test_binary():
    print(f"\033[1;34mRunning and testing binary: ./{BINARY_NAME} ...\033[0m")
    run_command = [f"./{BINARY_NAME}"]
    try:
        result = subprocess.run(run_command, check=True, capture_output=True, text=True)
        print("\033[1;32m✔ Binary executed successfully.\033[0m")
        print(f"\033[1m\033[36m\n--- Binary Output ---\033[0m\n{result.stdout}")
        if result.stderr:
            print(f"\033[1;33mStderr from binary: {result.stderr}\033[0m")

        # Simple check for test success message
        if "TestJisp: JSON parsing check passed." in result.stdout:
            print("\033[1;32m✔ TestJisp reported success.\033[0m")
            return True
        else:
            print("\033[1;31m❌ TestJisp did NOT report success. Check binary output for details.\033[0m")
            return False

    except subprocess.CalledProcessError as e:
        print("\033[1;31m❌ Binary execution failed.\033[0m")
        print(f"Stderr: {e.stderr}")
        print(f"Stdout: {e.stdout}")
        return False
    except FileNotFoundError:
        print(f"\033[1;31m❌ Binary '{BINARY_NAME}' not found. Did compilation succeed?\033[0m")
        return False

def clean_up():
    if os.path.exists(BINARY_NAME):
        os.remove(BINARY_NAME)
        print(f"\033[1;34mCleaned up: Removed {BINARY_NAME}.\033[0m")

if __name__ == "__main__":
    try:
        if compile_go_program():
            if run_and_test_binary():
                print("\033[1;32mAll tasks completed successfully! Go binary compiled and tests passed.\033[0m")
                sys.exit(0)
            else:
                print("\n\033[1;31m❌ Tests failed.\033[0m")
                sys.exit(1)
        else:
            print("\n\033[1;31m❌ Compilation failed.\033[0m")
            sys.exit(1)
    finally:
        clean_up()

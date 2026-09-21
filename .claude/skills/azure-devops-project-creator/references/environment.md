# Environment Setup and Teardown

Manages the Python virtual environment used during execution. The venv is created fresh
each run, used for the duration of the task, and removed on completion or failure.

---

## Setup

```python
import subprocess, sys, pathlib, shutil, os

VENV_DIR = pathlib.Path("/tmp/ado-creator-venv")

DEPENDENCIES = [
    "requests",
    "python-docx",
    "pymupdf",
    "openpyxl",
]

def setup_environment() -> pathlib.Path:
    """Creates the venv and installs dependencies. Returns the venv Python executable."""
    if VENV_DIR.exists():
        shutil.rmtree(VENV_DIR)

    subprocess.run(
        [sys.executable, "-m", "venv", str(VENV_DIR)],
        check=True, capture_output=True
    )

    pip = _pip_path()
    subprocess.run(
        [str(pip), "install", "--quiet", *DEPENDENCIES],
        check=True, capture_output=True
    )

    return _python_path()

def _python_path() -> pathlib.Path:
    if os.name == "nt":
        return VENV_DIR / "Scripts" / "python.exe"
    return VENV_DIR / "bin" / "python"

def _pip_path() -> pathlib.Path:
    if os.name == "nt":
        return VENV_DIR / "Scripts" / "pip.exe"
    return VENV_DIR / "bin" / "pip"
```

---

## Activating the Environment

Rather than activating the venv in the shell sense, import directly from it by prepending
its `site-packages` to `sys.path` before any dependency imports:

```python
import sys, pathlib, os

def activate_venv():
    """Prepends venv site-packages to sys.path so installed packages are importable."""
    if os.name == "nt":
        site_packages = VENV_DIR / "Lib" / "site-packages"
    else:
        # Python version-specific path (e.g. python3.11)
        lib = VENV_DIR / "lib"
        py_dir = next(lib.glob("python*"), None)
        if py_dir is None:
            raise RuntimeError("Could not locate site-packages in venv")
        site_packages = py_dir / "site-packages"

    if str(site_packages) not in sys.path:
        sys.path.insert(0, str(site_packages))
```

Call `activate_venv()` immediately after `setup_environment()` and before any imports of
`requests`, `docx`, `fitz`, or `openpyxl`.

---

## Teardown

Always called on completion, whether the task succeeded or failed. Use a `try/finally`
block to guarantee cleanup.

```python
def teardown_environment():
    """Removes the venv directory."""
    if not VENV_DIR.exists():
        return
    if os.name == "nt":
        # shutil.rmtree raises PermissionError on Windows when .pyd extension
        # modules are still locked by the running Python process. Use
        # cmd /c rmdir as the primary path; fall back to ignore_errors=True.
        import subprocess
        result = subprocess.run(
            ["cmd", "/c", "rmdir", "/s", "/q", str(VENV_DIR)],
            capture_output=True
        )
        if result.returncode != 0:
            shutil.rmtree(VENV_DIR, ignore_errors=True)
    else:
        shutil.rmtree(VENV_DIR)
    print(f"Environment cleaned up: {VENV_DIR}")
```

## load_credentials

Reads ADO credentials from environment variables. Never prompts for values in chat —
if variables are absent, explains how to set them and stops.

```python
import os

ADO_ORG_URL_VAR = "ADO_ORG_URL"
ADO_PAT_VAR     = "ADO_PAT"

def load_credentials() -> tuple[str, str]:
    """Returns (org_url, pat). Raises EnvironmentError with setup instructions if missing."""
    org_url = os.environ.get(ADO_ORG_URL_VAR)
    pat     = os.environ.get(ADO_PAT_VAR)

    missing = [v for v, val in {ADO_ORG_URL_VAR: org_url, ADO_PAT_VAR: pat}.items() if not val]
    if missing:
        instructions = "\n".join([
            f"Missing required environment variable(s): {', '.join(missing)}",
            "",
            "Set them before running (macOS / Linux):",
            f"  export {ADO_ORG_URL_VAR}=https://dev.azure.com/your-organisation",
            f"  export {ADO_PAT_VAR}=your-personal-access-token",
            "",
            "Set them before running (Windows):",
            f"  set {ADO_ORG_URL_VAR}=https://dev.azure.com/your-organisation",
            f"  set {ADO_PAT_VAR}=your-personal-access-token",
            "",
            "See references/api-patterns.md for how to create a PAT with the correct scopes.",
        ])
        raise EnvironmentError(instructions)

    return org_url, pat
```

---

### Usage pattern

```python
setup_environment()
activate_venv()
try:
    org_url, pat = load_credentials()
    # --- run the full skill workflow here ---
finally:
    teardown_environment()
```

---

## Notes

- The venv is always written to `/tmp/ado-creator-venv`. If this path is unavailable,
  fall back to `pathlib.Path.home() / ".ado-creator-venv"` and inform the user.
- If `setup_environment()` fails (e.g. no internet access, pip error), report the full
  subprocess output and stop — do not attempt to proceed without dependencies.
- The venv is not reused between runs. A fresh install ensures no stale package state.
#!/usr/bin/env python3
import json
import pathlib
import subprocess
import sys
import tempfile

binary = str(pathlib.Path(sys.argv[1]).resolve())
source = pathlib.Path(sys.argv[2]).read_bytes()
with tempfile.TemporaryDirectory(prefix="cyclo-parenthesized-blank-") as directory:
    root = pathlib.Path(directory)
    (root / "go.mod").write_text("module example.com/quality-repro\n\ngo 1.25.0\n")
    (root / "input.go").write_bytes(source)
    (root / "oracle_test.go").write_text('''package repro
import "testing"
func TestBlankWorks(t *testing.T) {
    if Blank() != 1 { t.Fatal("Blank must return 1") }
}
''')
    # Go 1.25 vet panics on parenthesized blank identifiers. Keep compilation
    # and runtime checks enabled, excluding that unrelated vet failure.
    runtime = subprocess.run(["go", "test", "-vet=off", "-count=1", "."], cwd=root, capture_output=True, timeout=20)
    if runtime.returncode != 0:
        sys.exit(1)
    analysis = subprocess.run([binary, "check", "--format", "facts", "input.go"], cwd=root, capture_output=True, timeout=20)
    if analysis.returncode != 0:
        sys.exit(1)
    try:
        function = next(f for f in json.loads(analysis.stdout)["functions"] if f["name"].endswith(".Blank"))
        lines = function["source"].splitlines()
        bug = any(m["root"] == "<temporary>" and "_" in lines[m["line"] - function["line"]] for m in function["mutations"])
    except (ValueError, KeyError, StopIteration, IndexError):
        sys.exit(1)
    if not bug:
        sys.exit(1)
    print("Same bug: discarding a value through a parenthesized blank identifier is counted as a mutation.")

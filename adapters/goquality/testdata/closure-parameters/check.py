#!/usr/bin/env python3
import json
import pathlib
import subprocess
import sys
import tempfile

binary = str(pathlib.Path(sys.argv[1]).resolve())
source = pathlib.Path(sys.argv[2]).read_bytes()
with tempfile.TemporaryDirectory(prefix="cyclo-closure-parameter-") as directory:
    root = pathlib.Path(directory)
    (root / "go.mod").write_text("module example.com/quality-repro\n\ngo 1.25.0\n")
    (root / "input.go").write_bytes(source)
    (root / "oracle_test.go").write_text('''package repro
import "testing"
func TestCallerStateChanges(t *testing.T) {
    shared := &State{}
    Mutate(shared)
    if shared.Count != 1 { t.Fatalf("caller count = %d, want 1", shared.Count) }
}
''')
    runtime = subprocess.run(["go", "test", "-count=1", "."], cwd=root, capture_output=True, timeout=20)
    if runtime.returncode != 0:
        sys.exit(1)
    analysis = subprocess.run([binary, "check", "--format", "facts", "input.go"], cwd=root, capture_output=True, timeout=20)
    if analysis.returncode != 0:
        sys.exit(1)
    try:
        facts = json.loads(analysis.stdout)
        function = next(f for f in facts["functions"] if f["name"].endswith(".Mutate"))
        bug = any(m["root"] == "value" and m["field_path"] == "Count" and m["provenance"] == "local" for m in function["mutations"])
    except (ValueError, KeyError, StopIteration):
        sys.exit(1)
    if not bug:
        sys.exit(1)
    print("Same bug: closure changes caller state, but its parameter-field mutation is classified local.")

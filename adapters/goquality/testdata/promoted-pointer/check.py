#!/usr/bin/env python3
import json
import pathlib
import subprocess
import sys
import tempfile

binary = str(pathlib.Path(sys.argv[1]).resolve())
source = pathlib.Path(sys.argv[2]).read_bytes()
with tempfile.TemporaryDirectory(prefix="cyclo-promoted-pointer-") as directory:
    root = pathlib.Path(directory)
    (root / "go.mod").write_text("module example.com/quality-repro\n\ngo 1.25.0\n")
    (root / "input.go").write_bytes(source)
    (root / "oracle_test.go").write_text("""package repro
import "testing"
func TestCallerStateChanges(t *testing.T) {
    shared := &State{}
    Mutate(shared)
    if shared.Count != 1 { t.Fatalf("caller count = %d, want 1", shared.Count) }
}
""")
    runtime = subprocess.run(["go", "test", "-count=1", "."], cwd=root, capture_output=True, timeout=20)
    if runtime.returncode != 0:
        sys.exit(1)
    analysis = subprocess.run([binary, "check", "--format", "json", "input.go"], cwd=root, capture_output=True, timeout=20)
    if analysis.returncode not in (0, 1):
        sys.exit(1)
    try:
        report = json.loads(analysis.stdout)
        function = next(f for f in report["functions"] if f["name"].endswith(".Mutate"))
    except (ValueError, KeyError, StopIteration):
        sys.exit(1)
    bug = function["mutations"] > 0 and function["complete"] and function["density_milli"] == 0 and not function["effects"]
    if not bug:
        sys.exit(1)
    print("Same bug: caller state changes, but quality reports complete with zero density and effects.")

#!/usr/bin/env python3
import json
import pathlib
import subprocess
import sys
import tempfile

binary = str(pathlib.Path(sys.argv[1]).resolve())
source = pathlib.Path(sys.argv[2]).read_bytes()
with tempfile.TemporaryDirectory(prefix="cyclo-generic-array-") as directory:
    root = pathlib.Path(directory)
    (root / "go.mod").write_text("module example.com/quality-repro\n\ngo 1.25.0\n")
    (root / "input.go").write_bytes(source)
    (root / "oracle_test.go").write_text('''package repro
import "testing"
func TestOnlyCopyChanges(t *testing.T) {
    shared := [1]int{}
    if Copy(shared) != 1 { t.Fatal("copy must change") }
    if shared[0] != 0 { t.Fatal("caller array must stay unchanged") }
}
''')
    runtime = subprocess.run(["go", "test", "-count=1", "."], cwd=root, capture_output=True, timeout=20)
    if runtime.returncode != 0:
        sys.exit(1)
    analysis = subprocess.run([binary, "check", "--format", "json", "input.go"], cwd=root, capture_output=True, timeout=20)
    if analysis.returncode not in (0, 1):
        sys.exit(1)
    try:
        function = next(f for f in json.loads(analysis.stdout)["functions"] if f["name"].endswith(".Copy"))
        bug = any(e["kind"] == "mutation" for e in function["effects"])
    except (ValueError, KeyError, StopIteration):
        sys.exit(1)
    if not bug:
        sys.exit(1)
    print("Same bug: only the generic array copy changes, but quality reports an external mutation effect.")

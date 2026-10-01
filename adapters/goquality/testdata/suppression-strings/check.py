#!/usr/bin/env python3
import json
import pathlib
import subprocess
import sys
import tempfile

binary = str(pathlib.Path(sys.argv[1]).resolve())
source = pathlib.Path(sys.argv[2]).read_bytes()
with tempfile.TemporaryDirectory(prefix="cyclo-suppression-string-") as directory:
    root = pathlib.Path(directory)
    (root / "go.mod").write_text("module example.com/quality-repro\n\ngo 1.25.0\n")
    (root / "input.go").write_bytes(source)
    (root / "oracle_test.go").write_text('''package repro
import "testing"
func TestPlainWorks(t *testing.T) {
    if Plain() != 1 { t.Fatal("Plain must return 1") }
}
''')
    runtime = subprocess.run(["go", "test", "-count=1", "."], cwd=root, capture_output=True, timeout=20)
    if runtime.returncode != 0:
        sys.exit(1)
    facts = subprocess.run([binary, "check", "--format", "facts", "input.go"], cwd=root, capture_output=True, timeout=20)
    analysis = subprocess.run([binary, "check", "--format", "json", "input.go"], cwd=root, capture_output=True, timeout=20)
    if facts.returncode != 0 or analysis.returncode != 1:
        sys.exit(1)
    try:
        function = next(f for f in json.loads(facts.stdout)["functions"] if f["name"].endswith(".Plain"))
        preceding = function["preceding_line"]
        diagnostics = json.loads(analysis.stdout)["diagnostics"]
        bug = preceding.startswith("const ") and "cyclo-allow" in preceding and any(d["name"].endswith(".Plain") and d["rule_id"] == "invalid_suppression" for d in diagnostics)
    except (ValueError, KeyError, StopIteration):
        sys.exit(1)
    if not bug:
        sys.exit(1)
    print("Same bug: a valid string constant triggers invalid_suppression on Plain.")

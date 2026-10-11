package pdgjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shanejonas/cyclo/domain/pdg"
)

func exportFixture(t *testing.T) *pdg.Graph {
	t.Helper()
	b := pdg.NewBuilder()
	policy := b.Policy("test", "1")
	evidence := b.Evidence(pdg.Approximated, "Approximate fixture.", policy)
	origin := b.AddProvenance(pdg.Provenance{Origin: pdg.Projection, Description: b.Text("Derived."), Policy: policy, Nodes: []pdg.Text{b.Text("prior-node")}, Spans: []pdg.Span{{Source: 1, End: 1}}})
	g := &pdg.Graph{
		Tables: b.Tables, Profile: policy,
		Producer:    pdg.Producer{Name: b.Text("test"), Version: b.Text("1"), Language: b.Text("go"), LanguageVersion: b.Missing(pdg.Unknown, "Unknown Go version."), Configuration: b.Text(`{"flag":true}`)},
		Sources:     []pdg.Source{{ID: b.Text("src"), Path: b.Text("a.go"), ContentIdentity: b.Missing(pdg.Unknown, "Not hashed.")}},
		Function:    pdg.Function{ID: b.Text("f"), Name: b.Known(""), QualifiedName: b.Known("pkg.f"), Span: pdg.Span{Source: 1, End: 10}, Interface: evidence, ReferenceCount: pdg.CountFact{Status: pdg.Known, Policy: policy}, Inputs: []pdg.Parameter{{Name: b.Known("arg"), Type: b.Known("*int"), Variadic: b.Known("false"), Symbol: 1}}},
		Symbols:     []pdg.Symbol{{ID: b.Text("s"), Scope: b.Text("scope"), Name: b.Known("arg"), Type: b.Known("*int"), Role: pdg.ParameterRole}},
		Locations:   []pdg.Location{{ID: b.Text("heap"), Abstraction: b.Text("test-region"), Description: b.Text("Storage."), Evidence: evidence}},
		Definitions: []pdg.Definition{{ID: b.Text("def"), Node: 1, Symbol: 1, Location: 1}},
		Nodes:       []pdg.Node{{ID: b.Text("n"), Category: pdg.Assignment, OriginalKind: b.Known("AssignStmt"), Span: 1, Attributes: 1, Evidence: evidence, Provenance: origin, Annotations: 1}},
		Spans:       []pdg.Span{{Source: 1, End: 5}},
		Annotations: [][]pdg.Extension{{{Name: b.Text("test:future"), Value: b.Text(`{"items":[1,null]}`)}}},
		Edges: []pdg.Edge{
			{ID: b.Text("e1"), Source: 1, Target: 1, Kind: pdg.Data, Subkind: b.Text("flow"), Policy: policy, Evidence: evidence, Provenance: origin, Symbol: 1, Definition: 1, Location: 1, Position: 1, LoopCarried: 1},
			{ID: b.Text("e2"), Source: 1, Target: 1, Kind: pdg.Execution, Subkind: b.Text("must-precede"), Policy: policy, Evidence: evidence, Provenance: origin, Position: 2, LoopCarried: 2},
		},
		Diagnostics: []pdg.Diagnostic{{Code: b.Text("test"), Message: b.Text("Incomplete."), Severity: pdg.Warning, Span: 1, Node: 1, Capability: b.Text("memory_aliases")}},
		Conformance: []pdg.Conformance{{Profile: policy, Status: pdg.NonConformant, Reasons: []pdg.Text{b.Text("Incomplete.")}}},
		Extensions:  []pdg.Extension{{Name: b.Text("test:root"), Value: b.Text(`false`)}},
	}
	for _, name := range pdg.RequiredCapabilities {
		g.Capabilities = append(g.Capabilities, pdg.NamedCapability{Name: b.Text(name), Evidence: evidence})
	}
	attrs := []pdg.Attributes{{Defines: []pdg.Ref{1}, Symbols: []pdg.Ref{1}, Reads: []pdg.Access{{Symbol: 1, Position: 1}, {UnresolvedReason: b.Text("Unknown target.")}}, Writes: []pdg.Access{{Location: 1}}, Position: 1, Operator: b.Known("="), Extensions: g.Extensions}}
	if err := pdg.PackAttributes(g, attrs); err != nil {
		t.Fatal(err)
	}
	return g
}

func exportedObject(t *testing.T, g *pdg.Graph) object {
	t.Helper()
	var out bytes.Buffer
	if err := Write(&out, g); err != nil {
		t.Fatal(err)
	}
	var doc object
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestExportPreservesFactsReferencesAndOccurrences(t *testing.T) {
	g := exportFixture(t)
	before, _ := json.Marshal(g)
	doc := exportedObject(t, g)
	function := doc["function"].(map[string]any)
	if got := function["name"]; !reflect.DeepEqual(got, object{"status": "known", "value": ""}) {
		t.Fatalf("known empty name: %v", got)
	}
	count := function["reference_count"].(map[string]any)
	if count["value"] != float64(0) || count["policy"] == nil {
		t.Fatalf("known zero: %v", count)
	}
	edges := doc["edges"].([]any)
	if len(edges) != 2 {
		t.Fatal("parallel self edges lost")
	}
	first, second := edges[0].(map[string]any), edges[1].(map[string]any)
	if first["source"] != "n" || first["definition_id"] != "def" || first["location_id"] != "heap" || first["symbol_id"] != "s" {
		t.Fatalf("unresolved references: %v", first)
	}
	if first["operand_position"] != float64(0) || first["loop_carried"] != false || second["loop_carried"] != true {
		t.Fatal("optional zero or false lost")
	}
	nodes := doc["nodes"].([]any)
	n := nodes[0].(map[string]any)
	if n["annotations"] == nil || n["provenance"] == nil || n["evidence"] == nil {
		t.Fatal("evidence or extensions lost")
	}
	if doc["extensions"].(map[string]any)["test:root"] != false {
		t.Fatal("extension JSON changed")
	}
	after, _ := json.Marshal(g)
	if !bytes.Equal(before, after) {
		t.Fatal("export mutated compact graph")
	}
}

func TestExportUnknownAndAbsentFacts(t *testing.T) {
	g := exportFixture(t)
	g.Function.ReferenceCount = pdg.CountFact{Status: pdg.Unknown, Reason: g.Producer.LanguageVersion.Value}
	g.Edges[0].Position = 0
	g.Edges[0].LoopCarried = 0
	g.Function.Outputs = nil
	doc := exportedObject(t, g)
	count := doc["function"].(map[string]any)["reference_count"].(map[string]any)
	if count["status"] != "unknown" || count["reason"] == nil {
		t.Fatal("unknown count lost")
	}
	if _, ok := count["value"]; ok {
		t.Fatal("unknown count became zero")
	}
	edge := doc["edges"].([]any)[0].(map[string]any)
	if _, ok := edge["operand_position"]; ok {
		t.Fatal("absent operand emitted")
	}
	if _, ok := edge["loop_carried"]; ok {
		t.Fatal("absent loop fact emitted")
	}
	if outputs := doc["function"].(map[string]any)["outputs"].([]any); outputs == nil || len(outputs) != 0 {
		t.Fatal("required empty array became null")
	}
}

var errWriter = errors.New("writer failed")

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errWriter }
func TestExportRejectsInvalidInputAndReturnsWriterError(t *testing.T) {
	var out bytes.Buffer
	if err := Write(&out, nil); err == nil || out.Len() != 0 {
		t.Fatal("invalid graph wrote output")
	}
	if err := Write(failedWriter{}, exportFixture(t)); !errors.Is(err, errWriter) {
		t.Fatalf("writer error: %v", err)
	}
}

// Optional external validation checks the complete fixture against the pinned
// schema, rather than reimplementing schema rules in the Go tests.
func TestExportMatchesCompatSchema(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is needed for external schema validation")
	}
	if err := exec.Command(python, "-c", "import jsonschema").Run(); err != nil {
		t.Skip("install Python jsonschema to run external schema validation")
	}
	var output bytes.Buffer
	output.WriteString("[")
	if err := Write(&output, exportFixture(t)); err != nil {
		t.Fatal(err)
	}
	output.WriteString("]")
	path := filepath.Join(t.TempDir(), "pdg.json")
	if err := os.WriteFile(path, output.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(python, "../../docs/pdg/validate-export.py", path)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("schema validation: %v\n%s", err, out)
	}
}

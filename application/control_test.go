package application_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/shanejonas/cyclo/application"
	"github.com/shanejonas/cyclo/domain"
)

func TestControlAPIAdvertisesItsRuntimeAddressAndMethods(t *testing.T) {
	server, _ := startControlTestServer(t)
	document := rpcResult(t, server.URL(), "rpc.discover", nil)

	if document["openrpc"] != "1.2.6" {
		t.Fatalf("openrpc = %v, want 1.2.6", document["openrpc"])
	}
	servers := document["servers"].([]any)
	local := servers[0].(map[string]any)
	if local["url"] != server.URL() {
		t.Fatalf("discovery URL = %v, want %s", local["url"], server.URL())
	}

	want := []string{
		"rpc.discover",
		"cyclo.getState",
		"cyclo.waitForChange",
		"cyclo.getReport",
		"cyclo.setFocus",
		"cyclo.selectFile",
		"cyclo.selectFunction",
		"cyclo.scrollSource",
		"cyclo.revealLines",
		"cyclo.clearLineSelection",
		"cyclo.annotateLines",
		"cyclo.removeAnnotation",
		"cyclo.refresh",
	}
	if got := methodNames(document); !sameStrings(got, want) {
		t.Fatalf("methods = %v, want %v", got, want)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte("cyclomaticDiagnostics")) || !bytes.Contains(encoded, []byte("cognitiveDiagnostics")) {
		t.Fatal("OpenRPC document does not describe complexity diagnostics")
	}
}

func TestControlAPIReadsAndChangesTheLiveModel(t *testing.T) {
	server, analyzer := startControlTestServer(t)

	state := rpcResult(t, server.URL(), "cyclo.getState", nil)
	assertSelection(t, state, "hot.go", "Branchy")
	revision := state["revision"]
	state = rpcResult(t, server.URL(), "cyclo.waitForChange", map[string]any{
		"afterRevision": revision,
		"timeoutMs":     0,
	})
	if state["revision"] != revision {
		t.Fatalf("zero-timeout wait revision = %v, want %v", state["revision"], revision)
	}

	state = rpcResult(t, server.URL(), "cyclo.setFocus", map[string]any{"pane": "functions"})
	if state["focus"] != "functions" {
		t.Fatalf("focus = %v, want functions", state["focus"])
	}

	state = rpcResult(t, server.URL(), "cyclo.selectFile", map[string]any{"index": 1})
	assertSelection(t, state, "calm.go", "Calm")

	state = rpcResult(t, server.URL(), "cyclo.selectFile", map[string]any{"index": 0})
	state = rpcResult(t, server.URL(), "cyclo.selectFunction", map[string]any{"index": 1})
	assertSelection(t, state, "hot.go", "Simple")

	state = rpcResult(t, server.URL(), "cyclo.scrollSource", map[string]any{"lines": 2})
	selection := state["selection"].(map[string]any)
	if selection["sourceOffset"] != float64(2) {
		t.Fatalf("sourceOffset = %v, want 2", selection["sourceOffset"])
	}

	state = rpcResult(t, server.URL(), "cyclo.revealLines", map[string]any{"startLine": 2, "endLine": 3})
	if state["focus"] != "source" || state["visualSelectionActive"] != false {
		t.Fatalf("reveal state = %v, want a non-visual source selection", state)
	}
	lineSelection := state["lineSelection"].(map[string]any)
	if lineSelection["startLine"] != float64(2) || lineSelection["endLine"] != float64(3) {
		t.Fatalf("lineSelection = %v, want lines 2-3", lineSelection)
	}

	annotationResult := rpcResult(t, server.URL(), "cyclo.annotateLines", map[string]any{
		"startLine": 2, "endLine": 3, "message": "Flatten this branch",
	})
	annotation := annotationResult["annotation"].(map[string]any)
	if annotation["message"] != "Flatten this branch" || annotation["text"] != "two\nthree" {
		t.Fatalf("annotation = %v", annotation)
	}
	state = annotationResult["state"].(map[string]any)
	if len(state["annotations"].([]any)) != 1 {
		t.Fatalf("annotations = %v, want one", state["annotations"])
	}

	state = rpcResult(t, server.URL(), "cyclo.clearLineSelection", nil)
	if _, ok := state["lineSelection"]; ok {
		t.Fatalf("line selection was not cleared: %v", state)
	}
	state = rpcResult(t, server.URL(), "cyclo.removeAnnotation", map[string]any{
		"annotationId": annotation["id"],
	})
	if len(state["annotations"].([]any)) != 0 {
		t.Fatalf("annotations after removal = %v", state["annotations"])
	}

	report := rpcResult(t, server.URL(), "cyclo.getReport", nil)
	if report["functions"] != float64(3) {
		t.Fatalf("report functions = %v, want 3", report["functions"])
	}
	if report["cognitiveTotal"] != float64(9) || report["cognitiveAverage"] != float64(3) {
		t.Fatalf("cognitive report = %v", report)
	}
	files := report["files"].([]any)
	functions := files[0].(map[string]any)["functions"].([]any)
	firstFunction := functions[0].(map[string]any)
	if _, ok := firstFunction["source"]; ok {
		t.Fatalf("report repeats function source: %v", functions[0])
	}
	if firstFunction["cognitiveComplexity"] != float64(8) {
		t.Fatalf("cognitive complexity = %v, want 8", firstFunction["cognitiveComplexity"])
	}
	if len(firstFunction["cyclomaticDiagnostics"].([]any)) != 1 {
		t.Fatalf("cyclomatic diagnostics = %v", firstFunction["cyclomaticDiagnostics"])
	}
	if len(firstFunction["cognitiveDiagnostics"].([]any)) != 1 {
		t.Fatalf("cognitive diagnostics = %v", firstFunction["cognitiveDiagnostics"])
	}

	state = rpcResult(t, server.URL(), "cyclo.refresh", nil)
	if analyzer.calls != 2 || state["refreshing"] != false {
		t.Fatalf("refresh calls = %d, state = %v", analyzer.calls, state)
	}
	if state["revision"].(float64) < 6 {
		t.Fatalf("revision = %v, want control mutations to advance it", state["revision"])
	}
}

func TestControlAPIRejectsInvalidRequests(t *testing.T) {
	server, _ := startControlTestServer(t)

	errorObject := rpcError(t, server.URL(), "cyclo.selectFile", map[string]any{"index": 99})
	if errorObject["code"] != float64(-32602) {
		t.Fatalf("error code = %v, want -32602", errorObject["code"])
	}

	errorObject = rpcError(t, server.URL(), "cyclo.nope", nil)
	if errorObject["code"] != float64(-32601) {
		t.Fatalf("error code = %v, want -32601", errorObject["code"])
	}
}

type controlTestAnalyzer struct {
	calls int
}

func (a *controlTestAnalyzer) Analyze([]string) (domain.Report, error) {
	a.calls++
	return controlTestReport(), nil
}

func startControlTestServer(t *testing.T) (*application.ControlServer, *controlTestAnalyzer) {
	t.Helper()

	analyzer := &controlTestAnalyzer{}
	model := application.NewModel(analyzer, []string{"/workspace"})
	model = runModelCommand(t, model, model.Init())

	server, err := application.NewControlServer(0)
	if err != nil {
		t.Fatal(err)
	}
	model = model.WithControlPort(server.Port())
	server.Start(func(message tea.Msg) {
		next, command := model.Update(message)
		model = next.(application.Model)
		model = runModelCommand(t, model, command)
	})
	t.Cleanup(func() { _ = server.Close() })

	return server, analyzer
}

func runModelCommand(t *testing.T, model application.Model, command tea.Cmd) application.Model {
	t.Helper()
	if command == nil {
		return model
	}

	message := command()
	next, followup := model.Update(message)
	if followup != nil {
		t.Fatal("unexpected follow-up command")
	}
	return next.(application.Model)
}

func controlTestReport() domain.Report {
	return domain.Report{
		Root: "/workspace",
		Files: []domain.File{
			{
				Path: "calm.go", Total: 1, Peak: 1, Average: 1,
				Functions: []domain.Function{{Package: "sample", Name: "Calm", Complexity: 1, Line: 1, EndLine: 3, Column: 1, Source: "func Calm() {\n\n}"}},
			},
			{
				Path: "hot.go", Total: 8, Peak: 6, Average: 4, CognitiveTotal: 9, CognitivePeak: 8, CognitiveAverage: 4.5,
				Functions: []domain.Function{
					{Package: "sample", Name: "Simple", Complexity: 2, CognitiveComplexity: 1, Line: 1, EndLine: 5, Column: 1, Source: "one\ntwo\nthree\nfour\nfive"},
					{Package: "sample", Name: "Branchy", Complexity: 6, CyclomaticDiagnostics: []domain.CyclomaticDiagnostic{{Kind: "function", Line: 7, Column: 1}}, CognitiveComplexity: 8, CognitiveDiagnostics: []domain.CognitiveDiagnostic{{Increment: 2, Nesting: 1, Kind: "if", Line: 8, Column: 2}}, Line: 7, EndLine: 10, Column: 1, Source: "one\ntwo\nthree\nfour"},
				},
			},
		},
		Functions:        3,
		Total:            9,
		Average:          3,
		CognitiveTotal:   9,
		CognitiveAverage: 3,
	}
}

func rpcResult(t *testing.T, url string, method string, params any) map[string]any {
	t.Helper()
	response := rpcRequest(t, url, method, params)
	result, ok := response["result"].(map[string]any)
	if !ok {
		t.Fatalf("%s result = %v", method, response)
	}
	return result
}

func rpcError(t *testing.T, url string, method string, params any) map[string]any {
	t.Helper()
	response := rpcRequest(t, url, method, params)
	errorObject, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("%s error = %v", method, response)
	}
	return errorObject
}

func rpcRequest(t *testing.T, url string, method string, params any) map[string]any {
	t.Helper()

	request := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		request["params"] = params
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	err = json.Unmarshal(payload, &envelope)
	if err != nil {
		t.Fatalf("decode %s response: %v\n%s", method, err, payload)
	}
	return envelope
}

func methodNames(document map[string]any) []string {
	methods := document["methods"].([]any)
	names := make([]string, 0, len(methods))
	for _, value := range methods {
		method := value.(map[string]any)
		names = append(names, method["name"].(string))
	}
	return names
}

func sameStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func assertSelection(t *testing.T, state map[string]any, path string, name string) {
	t.Helper()
	selection := state["selection"].(map[string]any)
	file := selection["file"].(map[string]any)
	function := selection["function"].(map[string]any)
	if file["path"] != path || function["name"] != name {
		t.Fatalf("selection = %v, want %s %s", selection, path, name)
	}
}

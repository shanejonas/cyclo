package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
)

type specimen struct {
	Name, Base, Family, Variant, Body, Signature, Argument string
	Changed, Clean                                         bool
}

type family struct {
	name, body, signature, argument string
	changed, clean                  bool
}

func families() []family {
	return []family{
		{name: "pointer-alias", body: "local := shared; local.Count++", changed: true},
		{name: "slice-alias", body: "local := shared.Items; local[0]++", changed: true},
		{name: "map-alias", body: `local := shared.Values; local["x"]++`, changed: true},
		{name: "array-copy", body: "local := shared.Array; local[0]++", clean: true},
		{name: "owned-pointer", body: "local := new(State); local.Count++", clean: true},
		{name: "owned-slice", body: "local := make([]int, 1); local[0]++", clean: true},
		{name: "owned-map", body: `local := make(map[string]int); local["x"]++`, clean: true},
		{name: "indirect-write", body: "local := new(State); holder := &local; *holder = shared; local.Count++", changed: true},
		{name: "range-write", body: "local := new(State); for _, local = range []*State{shared} {}; local.Count++", changed: true},
		{name: "embedded-pointer", body: "local := struct{*State}{shared}; local.Count++", changed: true},
		{name: "container-pointer", body: "local := []*State{shared}; local[0].Count++", changed: true},
		{name: "closure-parameter", body: "local := func(value *State) { if false { value = new(State) }; value.Count++ }; local(shared)", changed: true},
		{name: "pointer-rebind", body: "local := new(State); local = shared; local.Count++", changed: true},
		{name: "generic-pointer", body: "local := shared; (*local).Count++", signature: "[T ~*State](shared T)", changed: true},
		{name: "generic-slice", body: "local := shared; local[0]++", signature: "[T ~[]int](shared T)", argument: "shared.Items", changed: true},
		{name: "generic-array-copy", body: "local := shared; local[0]++", signature: "[T ~[1]int](shared T)", argument: "shared.Array", clean: true},
		{name: "generic-array-slice", body: "local := shared[:]; local[0]++", signature: "[T ~[1]int](shared T)", argument: "shared.Array", clean: true},
		{name: "generic-array-union", body: "local := shared; local[0]++", signature: "[T ~[1]int | ~[2]int](shared T)", argument: "shared.Array", clean: true},
		{name: "helper-write", body: "helperChain(shared)", changed: true},
		{name: "helper-pure", body: "local := helperPure(1); _ = local", clean: true},
		{name: "generic-owned-slice", body: "local := make(T, 1); local[0]++", signature: "[T ~[]int](shared T)", argument: "shared.Items", clean: true},
	}
}

func corpus() ([]specimen, error) {
	var result []specimen
	for index, f := range families() {
		base := fmt.Sprintf("Case%d_base", index)
		for _, variant := range []string{"base", "parentheses", "rename", "comments", "discard", "combined"} {
			body, err := variantBody(f.body, variant)
			if err != nil {
				return nil, err
			}
			result = append(result, specimen{Name: fmt.Sprintf("Case%d_%s", index, variant), Base: base, Family: f.name, Variant: variant,
				Body: body, Signature: f.signature, Argument: f.argument, Changed: f.changed, Clean: f.clean})
		}
	}
	return result, nil
}

func variantBody(body, variant string) (string, error) {
	switch variant {
	case "rename":
		return strings.ReplaceAll(body, "local", "ownedOrShared"), nil
	case "comments":
		return "// Ownership example; text is not an effect.\n" + body + "\n// End of example.\n", nil
	case "discard":
		return body + "; ((_)) = 0", nil
	case "parentheses":
		return parenthesized(body)
	case "combined":
		wrapped, err := parenthesized(body)
		return "// Combined syntax variant.\n" + strings.ReplaceAll(strings.TrimSpace(wrapped), "local", "ownedOrShared") + "; ((_)) = 0", err
	default:
		return body, nil
	}
}

func parenthesized(body string) (string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "variant.go", "package p\nfunc f() {"+body+"}", 0)
	if err != nil {
		return "", err
	}
	block := file.Decls[0].(*ast.FuncDecl).Body
	ast.Inspect(block, func(n ast.Node) bool { wrapPlaces(n); return true })
	var output bytes.Buffer
	if err := format.Node(&output, token.NewFileSet(), block); err != nil {
		return "", err
	}
	text := output.String()
	return text[1 : len(text)-1], nil
}

func wrapPlaces(n ast.Node) {
	switch n := n.(type) {
	case *ast.AssignStmt:
		wrapAssignment(n)
	case *ast.IncDecStmt:
		n.X = &ast.ParenExpr{X: n.X}
	case *ast.SelectorExpr:
		n.X = &ast.ParenExpr{X: n.X}
	case *ast.IndexExpr:
		n.X = &ast.ParenExpr{X: n.X}
	case *ast.CallExpr:
		n.Fun = &ast.ParenExpr{X: n.Fun}
	}
}

func program(cases []specimen) []byte {
	var output strings.Builder
	output.WriteString("package repro\ntype State struct { Count int; Items []int; Values map[string]int; Array [1]int }\n")
	output.WriteString("func helperWrite(shared *State) { shared.Count++ }\nfunc helperChain(shared *State) { helperWrite(shared) }\nfunc helperPure(value int) int { local := value; local++; return local }\n")
	for _, c := range cases {
		signature := c.Signature
		if signature == "" {
			signature = "(shared *State)"
		}
		fmt.Fprintf(&output, "func %s%s {\n%s\n}\n", c.Name, signature, c.Body)
	}
	return []byte(output.String())
}

func oracle(cases []specimen) []byte {
	var output strings.Builder
	output.WriteString("package main\nimport (\"encoding/json\"; \"os\"; repro \"example.com/quality-hunt\")\nfunc main() { results := map[string]bool{}\n")
	for _, c := range cases {
		argument := c.Argument
		if argument == "" {
			argument = "shared"
		}
		fmt.Fprintf(&output, "{ shared := &repro.State{Items:[]int{0}, Values:map[string]int{\"x\":0}}; repro.%s(%s); results[%q] = shared.Count != 0 || shared.Items[0] != 0 || shared.Values[\"x\"] != 0 || shared.Array[0] != 0 }\n", c.Name, argument, c.Name)
	}
	output.WriteString("if err := json.NewEncoder(os.Stdout).Encode(results); err != nil { panic(err) }\n}\n")
	return []byte(output.String())
}

func wrapAssignment(statement *ast.AssignStmt) {
	if statement.Tok != token.ASSIGN {
		return
	}
	for i, place := range statement.Lhs {
		statement.Lhs[i] = &ast.ParenExpr{X: place}
	}
}

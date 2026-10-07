package treesitter

import (
	"bytes"
	"context"
	"encoding/xml"
	"github.com/shanejonas/cyclo/adapters/reducer"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"testing"
)

func TestDecodeAndByteRanges(t *testing.T) {
	source := []byte("package p\nfunc café() {}\n")
	data := []byte(`<sources><source><source_file srow="0" scol="0" erow="2" ecol="0"><package_clause srow="0" scol="0" erow="0" ecol="9"/><function_declaration srow="1" scol="0" erow="1" ecol="15"/></source_file></source></sources>`)
	root, valid, err := decode(data, "", nil)
	if err != nil || !valid {
		t.Fatalf("decode: %v, %v", valid, err)
	}
	n := root.Children[1]
	span, err := deletion(n, lineOffsets(source), len(source))
	if err != nil {
		t.Fatal(err)
	}
	if string(source[span.Start:span.End]) != "func café() {}" {
		t.Fatalf("byte range: %q", source[span.Start:span.End])
	}
	if removable("source_file", "package_clause") || removable("expression_list", "call_expression") {
		t.Fatal("essential syntax selected")
	}
	if _, _, err := decode([]byte("not XML"), "no Go grammar", nil); err == nil {
		t.Fatal("missing grammar accepted")
	}
	if _, err := offset(lineOffsets(source), 1, 100, len(source)); err == nil {
		t.Fatal("out of bounds position accepted")
	}
	if _, err := deletion(node{XMLName: xml.Name{Local: "empty"}}, lineOffsets(source), len(source)); err == nil {
		t.Fatal("empty node accepted")
	}
}

func TestGoParserIntegration(t *testing.T) {
	library := os.Getenv("CYCLO_TEST_GO_PARSER")
	if library == "" {
		t.Skip("set CYCLO_TEST_GO_PARSER to run against an installed Go grammar")
	}
	if _, err := exec.LookPath("tree-sitter"); err != nil {
		t.Fatal(err)
	}
	p := GoParser{Context: context.Background(), Library: library}
	source := []byte("package p\nfunc noise() {}\nfunc keep() { println(1); println(2) }\n")
	spans, err := p.Deletions(source)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, span := range spans {
		if bytes.Equal(source[span.Start:span.End], []byte("println(1);")) {
			found = true
		}
	}
	if !found {
		t.Fatalf("same-line statement not selected: %+v", spans)
	}
	dependent := []byte("package p\nfunc keep() { scratch := 1; _ = scratch; println(2) }\n")
	reduced, err := reducer.ReduceSyntaxWithProgress(dependent, func(candidate []byte) (bool, error) {
		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "input.go", candidate, 0)
		if err != nil {
			t.Fatalf("syntax error reached checker: %v", err)
		}
		config := types.Config{}
		_, err = config.Check("p", files, []*ast.File{file}, nil)
		return err == nil && bytes.Contains(candidate, []byte("println(2)")), nil
	}, nil, p)
	if err != nil || bytes.Contains(reduced, []byte("scratch")) {
		t.Fatalf("dependent statements: %s, %v", reduced, err)
	}
	valid, err := p.Valid([]byte("package p\nfunc broken( {\n"))
	if err != nil || valid {
		t.Fatalf("syntax error: %v, %v", valid, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Context = ctx
	if _, err := p.Valid(source); err == nil {
		t.Fatal("cancellation ignored")
	}
}

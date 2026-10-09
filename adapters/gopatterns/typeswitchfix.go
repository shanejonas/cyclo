package gopatterns

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strings"

	"github.com/shanejonas/cyclo/domain/patterns"
)

// applyTypeSwitchFix replaces a type switch whose arms all call the same
// method on the case-bound value with a direct interface method call.
//
// Example:
//
//	switch v := x.(type) {
//	case Dog:
//		v.Speak()
//	case Cat:
//		v.Speak()
//	}
//
// becomes:
//
//	if v, ok := x.(interface{ Speak() }); ok {
//		v.Speak()
//	}
//
// The comma-ok form preserves the no-match behavior of a switch without a
// default clause. The interface signature is taken from the method
// declarations in the same file; all must exist and be identical, or the
// fix is skipped.
func applyTypeSwitchFix(spec *patterns.FixSpec, src []byte) ([]byte, error) {
	fset, f, err := parseSpec(spec, src)
	if err != nil {
		return nil, err
	}
	params, err := typeSwitchParams(spec)
	if err != nil {
		return nil, err
	}
	sw, err := validatedTypeSwitch(fset, f, spec, params)
	if err != nil {
		return nil, err
	}
	// Find the method signature from declarations in this file.
	sig, ok := interfaceMethodSig(f, fset, params.types, params.method)
	if !ok {
		return nil, fmt.Errorf("type_switch: cannot determine %s signature", params.method)
	}
	return formatTypeSwitchFix(fset, sw, src, params, sig)
}

// validatedTypeSwitch finds the type switch at the spec line and verifies
// its binding matches the spec.
func validatedTypeSwitch(fset *token.FileSet, f *ast.File, spec *patterns.FixSpec, params typeSwitchSpecParams) (*ast.TypeSwitchStmt, error) {
	sw := findTypeSwitchAtLine(fset, f, spec.Line)
	if sw == nil {
		return nil, fmt.Errorf("type_switch: no type switch at line %d", spec.Line)
	}
	if !bindingMatches(sw, params) {
		return nil, fmt.Errorf("type_switch: switch at line %d does not match spec", spec.Line)
	}
	return sw, nil
}

// formatTypeSwitchFix builds the replacement, applies it, and gofmts.
func formatTypeSwitchFix(fset *token.FileSet, sw *ast.TypeSwitchStmt, src []byte, params typeSwitchSpecParams, sig string) ([]byte, error) {
	repl := buildTypeSwitchRepl(params, sig)
	swStart := fset.Position(sw.Pos()).Offset
	swEnd := fset.Position(sw.End()).Offset
	out := applyEdits(src, []textEdit{{start: swStart, end: swEnd, replacement: []byte(repl)}})
	formatted, err := format.Source(out)
	if err != nil {
		return nil, fmt.Errorf("gofmt after type_switch fix: %w", err)
	}
	return formatted, nil
}

// typeSwitchSpecParams are the FixSpec params for a type_switch candidate.
type typeSwitchSpecParams struct {
	bound  string
	expr   string
	method string
	types  []string
	args   string
}

// typeSwitchParams extracts and validates the FixSpec params.
func typeSwitchParams(spec *patterns.FixSpec) (typeSwitchSpecParams, error) {
	p := typeSwitchSpecParams{
		bound:  spec.Params["bound"],
		expr:   spec.Params["expr"],
		method: spec.Params["method"],
		types:  strings.Split(spec.Params["types"], ","),
		args:   spec.Params["args"],
	}
	if p.bound == "" || p.expr == "" || p.method == "" || len(p.types) < 2 {
		return p, fmt.Errorf("type_switch: incomplete FixSpec params")
	}
	return p, nil
}

// bindingMatches reports whether the switch's binding matches the spec.
func bindingMatches(sw *ast.TypeSwitchStmt, params typeSwitchSpecParams) bool {
	gotBound, gotExpr, ok := typeSwitchBinding(sw)
	return ok && gotBound == params.bound && gotExpr == params.expr
}

// interfaceMethodSig finds `func (r Type) Method(` for each type in the
// same file and returns the shared signature `Method(params) results`.
// All declarations must exist and have identical signatures.
func interfaceMethodSig(f *ast.File, fset *token.FileSet, types []string, method string) (string, bool) {
	var sig string
	for i, typ := range types {
		s, ok := methodSig(f, fset, typ, method)
		if !ok {
			return "", false
		}
		if i == 0 {
			sig = s
		} else if s != sig {
			return "", false
		}
	}
	return sig, sig != ""
}

// buildTypeSwitchRepl generates the if-with-type-assertion replacement.
// No trailing newline: the source after the switch already has one.
func buildTypeSwitchRepl(params typeSwitchSpecParams, sig string) string {
	call := params.bound + "." + params.method + "(" + params.args + ")"
	return fmt.Sprintf("if %s, ok := %s.(interface{ %s }); ok {\n\t%s\n}",
		params.bound, params.expr, sig, call)
}

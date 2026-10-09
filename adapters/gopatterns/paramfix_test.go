package gopatterns

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func fixParam(t *testing.T, src string) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, fixes, err := FixParameterize(fset, f, []byte(src), nil)
	if err != nil {
		t.Fatalf("FixParameterize: %v", err)
	}
	if len(fixes) == 0 {
		t.Fatal("expected fixes, got none")
	}
	return string(out)
}

func fixParamNone(t *testing.T, src string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, fixes, err := FixParameterize(fset, f, []byte(src), nil)
	if err != nil {
		t.Fatalf("FixParameterize: %v", err)
	}
	if len(fixes) != 0 {
		t.Fatalf("expected no fixes, got %d", len(fixes))
	}
}

func TestParamFixRetryPair(t *testing.T) {
	src := `package p

func FetchWithRetry(url string) (string, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		body, err := fetchURL(url)
		if err == nil {
			return body, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func LoadWithRetry(key string) (string, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		value, err := loadKey(key)
		if err == nil {
			return value, nil
		}
		lastErr = err
	}
	return "", lastErr
}
`
	out := fixParam(t, src)
	// Helper extracted with withRetry name (common suffix).
	if !strings.Contains(out, "func withRetry(fn func(string) (string, error), url string) (string, error)") {
		t.Errorf("helper not found in:\n%s", out)
	}
	// Both wrappers delegate.
	if !strings.Contains(out, "return withRetry(fetchURL, url)") {
		t.Errorf("FetchWithRetry wrapper not found in:\n%s", out)
	}
	if !strings.Contains(out, "return withRetry(loadKey, key)") {
		t.Errorf("LoadWithRetry wrapper not found in:\n%s", out)
	}
	// Original bodies gone (only one copy of the loop remains, in the helper).
	if strings.Count(out, "for i := 0; i < 3; i++") != 1 {
		t.Errorf("expected exactly one loop (in helper), got:\n%s", out)
	}
}

func TestParamFixIdempotent(t *testing.T) {
	src := `package p

func withRetry(fn func(string) (string, error), url string) (string, error) {
	return fn(url)
}

func FetchWithRetry(url string) (string, error) {
	return withRetry(fetchURL, url)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out1, fixes1, err := FixParameterize(fset, f, []byte(src), nil)
	if err != nil {
		t.Fatalf("FixParameterize: %v", err)
	}
	if len(fixes1) == 0 {
		t.Skip("no fixes on first pass, nothing to check idempotency for")
	}
	// Second pass on the output should be a no-op.
	fset2 := token.NewFileSet()
	f2, err := parser.ParseFile(fset2, "test.go", out1, parser.ParseComments)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	_, fixes2, err := FixParameterize(fset2, f2, out1, nil)
	if err != nil {
		t.Fatalf("FixParameterize: %v", err)
	}
	if len(fixes2) != 0 {
		t.Errorf("not idempotent: second pass found %d fixes", len(fixes2))
	}
}

func TestParamFixSkipsDifferentSignatures(t *testing.T) {
	src := `package p

func A(x string) string { return x }
func B(x int) string { return "b" }
`
	fixParamNone(t, src)
}

func TestParamFixSkipsClosures(t *testing.T) {
	src := `package p

func A(x string) string {
	f := func() string { return x }
	return f()
}

func B(y string) string {
	g := func() string { return y }
	return g()
}
`
	fixParamNone(t, src)
}

func TestParamFixSkipsNonAdjacent(t *testing.T) {
	src := `package p

func A(x string) string { return x }

func unrelated() {}

func B(y string) string { return y }
`
	fixParamNone(t, src)
}

func TestParamFixSkipsTooManyHoles(t *testing.T) {
	src := `package p

func A() string { return foo() + bar() + baz() + qux() }

func B() string { return foo2() + bar2() + baz2() + qux2() }
`
	// 4 holes > maxParamHoles (3), should skip.
	fixParamNone(t, src)
}

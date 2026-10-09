package gopatterns

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestConsequenceNaming(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"error return", `package p
import "net/http"
import "fmt"
func f(response *http.Response) (string, error) {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("bad: %s", response.Status)
	}
	return "ok", nil
}`, "IsErrorResponse"},
		{"happy path guard", `package p
type Answer struct{ Choice string; Confidence float64 }
func f(answer Answer) string {
	if answer.Choice != "none" && answer.Confidence >= 0.70 {
		return answer.Choice
	}
	return ""
}`, "IsValidAnswer"},
		{"simple structural", `package p
type User struct{ Age int }
func f(u User) bool {
	if u.Age > 18 {
		return true
	}
	return false
}`, "IsAgeOver18"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "test.go", tc.src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			var ifStmt *ast.IfStmt
			ast.Inspect(f, func(n ast.Node) bool {
				if s, ok := n.(*ast.IfStmt); ok && ifStmt == nil {
					ifStmt = s
				}
				return ifStmt == nil
			})
			got := predicateName(ifStmt.Cond, ifStmt)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

package gopatterns

import (
	"strings"
	"testing"

	"github.com/shanejonas/cyclo/domain/patterns"
)

func TestApplyFactoryFix(t *testing.T) {
	src := `package p

type Config struct {
	Host string
	Port int
	User string
	Pass string
	Name string
	DB   string
}

func a() {
	c := Config{Host: "h", Port: 1, User: "u", Pass: "p", Name: "n", DB: "d"}
	_ = c
}

func b() {
	c := Config{Name: "m", DB: "z", Host: "x", Port: 2, User: "v", Pass: "w"}
	_ = c
}
`
	spec := &patterns.FixSpec{
		Kind: patterns.Factory,
		File: "test.go",
		Line: 14,
		Params: map[string]string{
			"type": "Config",
		},
	}
	out, err := applyFactoryFix(spec, []byte(src))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "func NewConfig(") {
		t.Error("factory function not created")
	}
	if !strings.Contains(got, "NewConfig(\"h\", 1, \"u\", \"p\", \"n\", \"d\")") {
		t.Errorf("first literal not rewritten correctly:\n%s", got)
	}
	// Second literal has fields in different order; args must follow
	// declaration order: Host, Port, User, Pass, Name, DB.
	if !strings.Contains(got, "NewConfig(\"x\", 2, \"v\", \"w\", \"m\", \"z\")") {
		t.Errorf("second literal not rewritten in field order:\n%s", got)
	}
	if strings.Contains(got, `c := Config{Host:`) {
		t.Error("old literal form still present")
	}
}

func TestApplyFactoryFixSkipsPartialLiterals(t *testing.T) {
	src := `package p

type Config struct {
	Host string
	Port int
	User string
	Pass string
	Name string
	DB   string
}

func a() {
	c := Config{Host: "h", Port: 1}
	_ = c
}
`
	spec := &patterns.FixSpec{
		Kind:   patterns.Factory,
		File:   "test.go",
		Line:   13,
		Params: map[string]string{"type": "Config"},
	}
	out, err := applyFactoryFix(spec, []byte(src))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if strings.Contains(string(out), "func NewConfig(") {
		t.Error("factory should not be created when no literal sets all fields")
	}
}

func TestApplyFactoryFixSkipsExistingFactory(t *testing.T) {
	src := `package p

type Config struct {
	Host string
	Port int
	User string
	Pass string
	Name string
	DB   string
}

func NewConfig(host string, port int, user string, pass string, name string, db string) Config {
	return Config{Host: host, Port: port, User: user, Pass: pass, Name: name, DB: db}
}

func a() {
	c := Config{Host: "h", Port: 1, User: "u", Pass: "p", Name: "n", DB: "d"}
	_ = c
}
`
	spec := &patterns.FixSpec{
		Kind:   patterns.Factory,
		File:   "test.go",
		Line:   17,
		Params: map[string]string{"type": "Config"},
	}
	out, err := applyFactoryFix(spec, []byte(src))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if strings.Count(string(out), "func NewConfig(") != 1 {
		t.Error("should not create a duplicate factory")
	}
}

func TestApplyFactoryFixHandlesAddressOf(t *testing.T) {
	src := `package p

type Config struct {
	Host string
	Port int
	User string
	Pass string
	Name string
	DB   string
}

func a() *Config {
	return &Config{Host: "h", Port: 1, User: "u", Pass: "p", Name: "n", DB: "d"}
}

func b() *Config {
	return &Config{Host: "x", Port: 2, User: "v", Pass: "w", Name: "m", DB: "z"}
}
`
	spec := &patterns.FixSpec{
		Kind:   patterns.Factory,
		File:   "test.go",
		Line:   14,
		Params: map[string]string{"type": "Config"},
	}
	out, err := applyFactoryFix(spec, []byte(src))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "return &NewConfig(") {
		t.Errorf("&T{...} should become &NewT(...):\n%s", got)
	}
}

func TestFactoryFixRegistered(t *testing.T) {
	if _, ok := appliers[patterns.Factory]; !ok {
		t.Error("Factory not registered in appliers")
	}
}

package gopatterns

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// selfTyFixture has methods on two types plus a free function.
const selfTyFixture = `package p

type Dog struct{}

func (d Dog) Bark() string { return "woof" }

type Cat struct{}

func (c Cat) Meow() string { return "meow" }

func Helper() string { return "help" }
`

// TestExtractSelfTy verifies the extractor records the receiver's named
// type on FuncPdg.SelfTy for methods and leaves it empty for free
// functions. The miner needs SelfTy: signature groups require two or more
// distinct self types, and facts without one are skipped entirely.
func TestExtractSelfTy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/selfty\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(selfTyFixture), 0644); err != nil {
		t.Fatalf("p.go: %v", err)
	}
	ext, err := Extract(context.Background(), dir, nil)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	got := map[string]string{}
	for _, f := range ext.Funcs {
		got[f.Name] = f.SelfTy
	}
	for name, want := range map[string]string{
		"example.com/selfty.Dog.Bark": "example.com/selfty.Dog",
		"example.com/selfty.Cat.Meow": "example.com/selfty.Cat",
		"example.com/selfty.Helper":   "",
	} {
		if got[name] != want {
			t.Errorf("SelfTy[%s] = %q, want %q", name, got[name], want)
		}
	}
}

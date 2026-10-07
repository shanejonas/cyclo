package reducer

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"testing"
)

type testSyntax struct {
	deletions func([]byte) ([]Deletion, error)
	valid     func([]byte) (bool, error)
}

func (s testSyntax) Deletions(source []byte) ([]Deletion, error) { return s.deletions(source) }
func (s testSyntax) Valid(source []byte) (bool, error) {
	if s.valid != nil {
		return s.valid(source)
	}
	_, err := parser.ParseFile(token.NewFileSet(), "input.go", source, parser.AllErrors)
	return err == nil, nil
}

func TestSyntaxAttemptAcceptsNonNilEmptyCandidate(t *testing.T) {
	source := []byte("entire unit")
	syntax := testSyntax{valid: func(candidate []byte) (bool, error) {
		if candidate == nil || len(candidate) != 0 {
			t.Fatalf("candidate = %#v", candidate)
		}
		return true, nil
	}}
	candidate, accepted, err := syntaxAttempt(source, Deletion{End: len(source), Unit: "all"}, syntax,
		func(progress Progress) (bool, error) {
			return progress.Candidate != nil && len(progress.Candidate) == 0, nil
		})
	if err != nil || !accepted || candidate == nil || len(candidate) != 0 {
		t.Fatalf("candidate = %#v, accepted = %v, error = %v", candidate, accepted, err)
	}
	if string(source) != "entire unit" {
		t.Fatal("source changed")
	}
}

func TestSyntaxReductionNilSeedResultDistinguishesAcceptance(t *testing.T) {
	failure := errors.New("syntax unavailable")
	for _, tc := range []struct {
		name     string
		accepted bool
		seedErr  error
		passErr  error
		wantBest bool
	}{
		{name: "accepted", accepted: true, wantBest: true},
		{name: "rejected"},
		{name: "checker error", seedErr: failure},
		{name: "accepted before syntax error", accepted: true, passErr: failure, wantBest: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			syntax := testSyntax{
				valid: func(source []byte) (bool, error) {
					if source != nil {
						t.Fatal("seed syntax check must receive original nil input")
					}
					return true, nil
				},
				deletions: func(best []byte) ([]Deletion, error) {
					if best == nil || len(best) != 0 {
						t.Fatalf("accepted best = %#v", best)
					}
					return nil, tc.passErr
				},
			}
			observations := 0
			best, err := ReduceSyntaxWithProgress(nil, func(source []byte) (bool, error) {
				if source != nil {
					t.Fatal("checker must receive original nil seed")
				}
				return tc.accepted, tc.seedErr
			}, func(progress Progress) {
				observations++
				if !progress.Seed || progress.Candidate != nil {
					t.Fatalf("seed progress = %+v", progress)
				}
			}, syntax)
			if (best != nil) != tc.wantBest || len(best) != 0 || observations != 2 {
				t.Fatalf("best = %#v, observations = %d", best, observations)
			}
			if wantErr := !tc.accepted || tc.seedErr != nil || tc.passErr != nil; (err != nil) != wantErr {
				t.Fatalf("error = %v, want error = %v", err, wantErr)
			}
			if (tc.seedErr != nil || tc.passErr != nil) && !errors.Is(err, failure) {
				t.Fatalf("error = %v, want %v", err, failure)
			}
		})
	}
}

func TestSyntaxReductionReparsesAndSkipsMalformedCandidates(t *testing.T) {
	source := []byte("package p\nfunc noise() {}\nfunc keep() { println(1); println(2) }\n")
	original := bytes.Clone(source)
	syntax := testSyntax{deletions: func(current []byte) ([]Deletion, error) {
		result := []Deletion{{Start: 0, End: 7, Unit: "malformed"}}
		for _, text := range []string{"func noise() {}", "println(1); ", "println(2)"} {
			start := bytes.Index(current, []byte(text))
			if start >= 0 {
				result = append(result, Deletion{Start: start, End: start + len(text), Unit: "statement"})
			}
		}
		return result, nil
	}}
	checks := 0
	best := source
	reduced, err := ReduceSyntaxWithProgress(source, func(candidate []byte) (bool, error) {
		checks++
		if valid, _ := syntax.Valid(candidate); !valid {
			t.Fatal("malformed candidate reaches checker")
		}
		return bytes.Contains(candidate, []byte("println(2)")), nil
	}, func(p Progress) {
		if p.Seed || p.Checking {
			return
		}
		rebuilt := append(bytes.Clone(best[:p.StartByte]), best[p.EndByte:]...)
		if !bytes.Equal(rebuilt, p.Candidate) {
			t.Fatal("progress does not describe deletion")
		}
		if p.Accepted {
			best = p.Candidate
		}
	}, syntax)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reduced, []byte("noise")) || bytes.Contains(reduced, []byte("println(1)")) {
		t.Fatalf("not reduced: %s", reduced)
	}
	if checks != 4 {
		t.Fatalf("checks = %d, want 4", checks)
	}
	if !bytes.Equal(source, original) {
		t.Fatal("original changed")
	}
}

func TestSyntaxReductionPreservesBestOnFailure(t *testing.T) {
	source := []byte("package p\nfunc noise() {}\n")
	failure := errors.New("parser unavailable")
	passes := 0
	syntax := testSyntax{deletions: func(current []byte) ([]Deletion, error) {
		passes++
		if passes == 2 {
			return nil, failure
		}
		return []Deletion{{Start: 10, End: len(current), Unit: "function_declaration"}}, nil
	}}
	reduced, err := ReduceSyntaxWithProgress(source, func([]byte) (bool, error) { return true, nil }, nil, syntax)
	if !errors.Is(err, failure) || string(reduced) != "package p\n" {
		t.Fatalf("got %q, %v", reduced, err)
	}
	reduced, err = ReduceSyntaxWithProgress([]byte("invalid"), func([]byte) (bool, error) { t.Fatal("checker called on invalid seed"); return true, nil }, nil, syntax)
	if reduced != nil || err == nil {
		t.Fatalf("invalid seed: %q, %v", reduced, err)
	}
}

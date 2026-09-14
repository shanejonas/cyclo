package reducer

import (
	"bytes"
	"errors"
	"testing"
)

func TestReductionKeepsRequiredLines(t *testing.T) {
	source := []byte("noise\nfirst\nmore noise\nlast")
	reduced, err := Reduce(source, func(candidate []byte) (bool, error) {
		return bytes.Contains(candidate, []byte("first")) && bytes.Contains(candidate, []byte("last")), nil
	})
	if err != nil || string(reduced) != "first\nlast" {
		t.Fatalf("result = %q, %v", reduced, err)
	}
	if string(source) != "noise\nfirst\nmore noise\nlast" {
		t.Fatal("mutated input")
	}
}

func TestReductionReturnsBestOnCheckerError(t *testing.T) {
	want := errors.New("checker unavailable")
	calls := 0
	reduced, err := Reduce([]byte("first\nlast\n"), func(candidate []byte) (bool, error) {
		calls++
		if calls == 4 {
			return false, want
		}
		return bytes.Contains(candidate, []byte("last")), nil
	})
	if !errors.Is(err, want) || !bytes.Contains(reduced, []byte("last")) {
		t.Fatalf("result = %q, %v", reduced, err)
	}
}

func TestEmptyAcceptedResultIsNotFailure(t *testing.T) {
	reduced, err := Reduce([]byte("noise"), func([]byte) (bool, error) { return true, nil })
	if err != nil || reduced == nil || len(reduced) != 0 {
		t.Fatalf("result = %#v, %v", reduced, err)
	}
}

func TestAcceptedEmptyInputSurvivesCheckerError(t *testing.T) {
	want := errors.New("checker stopped")
	calls := 0
	reduced, err := Reduce([]byte{}, func([]byte) (bool, error) {
		calls++
		if calls == 1 {
			return true, nil
		}
		return false, want
	})
	if !errors.Is(err, want) || reduced == nil || len(reduced) != 0 {
		t.Fatalf("lost accepted empty result: %#v, %v", reduced, err)
	}
}

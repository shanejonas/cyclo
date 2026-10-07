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

func TestProgressDescribesChecksAndAcceptedDeletions(t *testing.T) {
	source := []byte("noise\nbug\nmore noise\n")
	best := source
	var started *Progress
	checks := 0
	result, err := ReduceWithProgress(source, func(candidate []byte) (bool, error) {
		return bytes.Contains(candidate, []byte("bug")), nil
	}, func(p Progress) {
		if p.Checking {
			if started != nil {
				t.Fatal("overlapping checks")
			}
			started = &p
			return
		}
		if started == nil || !bytes.Equal(started.Candidate, p.Candidate) {
			t.Fatal("completion without matching start")
		}
		started = nil
		checks++
		if p.Seed {
			if checks != 1 || !bytes.Equal(p.Candidate, source) {
				t.Fatal("seed must come first")
			}
			return
		}
		lines := bytes.SplitAfter(best, []byte("\n"))
		want := append([][]byte(nil), lines[:p.StartLine]...)
		want = append(want, lines[p.StartLine+p.RemovedLines:]...)
		if !bytes.Equal(bytes.Join(want, nil), p.Candidate) || p.ChunkSize < p.RemovedLines {
			t.Fatalf("incorrect deletion metadata: %+v", p)
		}
		if p.Accepted {
			best = p.Candidate
		}
	})
	if err != nil || string(result) != "bug\n" || checks < 3 || started != nil {
		t.Fatalf("result = %q, error = %v, checks = %d", result, err, checks)
	}
}

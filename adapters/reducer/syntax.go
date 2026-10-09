package reducer

import (
	"bytes"
	"fmt"
)

// Deletion identifies one complete syntax unit in the current source.
type Deletion struct {
	Start, End int
	Unit       string
}

// Syntax supplies byte ranges and rejects malformed candidates before checking.
type Syntax interface {
	Deletions([]byte) ([]Deletion, error)
	Valid([]byte) (bool, error)
}

// ReduceSyntaxWithProgress tries syntax units largest first, reparsing after
// every accepted deletion. Errors preserve the last checker-approved input.
// A nil result means the seed was never accepted; accepted empty inputs are non-nil.
func ReduceSyntaxWithProgress(source []byte, predicate func([]byte) (bool, error), observe func(Progress), syntax Syntax) ([]byte, error) {
	check := func(p Progress) (bool, error) { return checkProgress(p, predicate, observe) }
	if err := syntaxSeed(source, syntax, check); err != nil {
		return nil, err
	}
	best := source
	if best == nil {
		best = []byte{}
	}
	for {
		candidate, accepted, err := syntaxPass(best, syntax, check)
		if err != nil {
			return best, err
		}
		if !accepted {
			return best, nil
		}
		best = candidate
	}
}

func syntaxSeed(source []byte, syntax Syntax, check func(Progress) (bool, error)) error {
	valid, err := syntax.Valid(source)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("Tree-sitter rejects input syntax")
	}
	accepted, err := check(Progress{Candidate: source, Seed: true})
	if err != nil {
		return err
	}
	if !accepted {
		return fmt.Errorf("input does not satisfy the checker")
	}
	return nil
}

func syntaxPass(source []byte, syntax Syntax, check func(Progress) (bool, error)) ([]byte, bool, error) {
	deletions, err := syntax.Deletions(source)
	if err != nil {
		return nil, false, err
	}
	for _, deletion := range deletions {
		candidate, accepted, err := syntaxAttempt(source, deletion, syntax, check)
		if err != nil || accepted {
			return candidate, accepted, err
		}
	}
	return nil, false, nil
}

func syntaxAttempt(source []byte, deletion Deletion, syntax Syntax, check func(Progress) (bool, error)) ([]byte, bool, error) {
	if deletion.Start < 0 || deletion.End > len(source) || deletion.Start >= deletion.End {
		return nil, false, fmt.Errorf("invalid syntax deletion range: %d:%d", deletion.Start, deletion.End)
	}
	candidate := bytes.Join([][]byte{source[:deletion.Start], source[deletion.End:]}, nil)
	valid, err := syntax.Valid(candidate)
	if err != nil || !valid {
		return nil, false, err
	}
	accepted, err := check(Progress{Candidate: candidate, Unit: deletion.Unit,
		StartByte: deletion.Start, EndByte: deletion.End,
		StartLine: bytes.Count(source[:deletion.Start], []byte("\n")),
	})
	return candidate, accepted, err
}

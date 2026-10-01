package reducer

import (
	"bytes"
	"fmt"
	"slices"
)

// Progress describes a checker attempt. Candidate is read-only. Checking is
// true before the predicate runs; Accepted and Err are set when it returns.
// StartLine is zero-based, and ChunkSize identifies the line deletion pass.
// A nonempty Unit identifies syntax reduction; StartByte:EndByte is the removed
// half-open byte range in the last accepted source.
type Progress struct {
	Candidate          []byte
	StartByte, EndByte int
	Unit               string
	ChunkSize          int
	StartLine          int
	RemovedLines       int
	Seed               bool
	Checking           bool
	Accepted           bool
	Err                error
}

// Reduce removes chunks of whole lines, then repeats until no line can be
// deleted while preserving the predicate. It does not promise a global minimum.
// After the seed is accepted, errors return the best accepted input alongside
// the error. A nil result means the seed was never accepted.
func Reduce(source []byte, predicate func([]byte) (bool, error)) ([]byte, error) {
	return ReduceWithProgress(source, predicate, nil)
}

// ReduceWithProgress behaves like Reduce and synchronously reports each check.
// Observers must not mutate candidates or retain mutable state across goroutines.
func ReduceWithProgress(source []byte, predicate func([]byte) (bool, error), observe func(Progress)) ([]byte, error) {
	check := func(progress Progress) (bool, error) { return checkProgress(progress, predicate, observe) }
	ok, err := check(Progress{Candidate: source, Seed: true})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("input does not satisfy the checker")
	}
	lines := bytes.SplitAfter(source, []byte("\n"))
	for size := len(lines); size > 0; size /= 2 {
		lines, err = removeChunks(lines, size, check)
		if err != nil {
			break
		}
	}
	result := bytes.Join(lines, nil)
	// nil means the seed was rejected; an accepted empty input is non-nil.
	if result == nil {
		result = []byte{}
	}
	return result, err
}

func removeChunks(lines [][]byte, size int, check func(Progress) (bool, error)) ([][]byte, error) {
	for start := 0; start < len(lines); {
		end := min(start+size, len(lines))
		candidate := slices.Concat(lines[:start], lines[end:])
		ok, err := check(Progress{
			Candidate: bytes.Join(candidate, nil), ChunkSize: size,
			StartLine: start, RemovedLines: end - start,
		})
		if err != nil {
			return lines, err
		}
		if !ok {
			start++
			continue
		}
		lines = candidate
		start = 0
	}
	return lines, nil
}

func checkProgress(progress Progress, predicate func([]byte) (bool, error), observe func(Progress)) (bool, error) {
	if observe != nil {
		progress.Checking = true
		observe(progress)
	}
	accepted, err := predicate(progress.Candidate)
	if observe != nil {
		progress.Checking, progress.Accepted, progress.Err = false, accepted, err
		observe(progress)
	}
	return accepted, err
}

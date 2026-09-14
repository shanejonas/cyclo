package reducer

import (
	"bytes"
	"fmt"
)

// Reduce removes chunks of whole lines, then repeats until no line can be
// deleted while preserving the predicate. It does not promise a global minimum.
// After the seed is accepted, errors return the best accepted input alongside
// the error. A nil result means the seed was never accepted.
func Reduce(source []byte, predicate func([]byte) (bool, error)) ([]byte, error) {
	ok, err := predicate(source)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("input does not satisfy the checker")
	}
	lines := bytes.SplitAfter(source, []byte("\n"))
	for size := len(lines); size > 0; size /= 2 {
		lines, err = removeChunks(lines, size, predicate)
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

func removeChunks(lines [][]byte, size int, predicate func([]byte) (bool, error)) ([][]byte, error) {
	for start := 0; start < len(lines); {
		candidate := append([][]byte(nil), lines[:start]...)
		candidate = append(candidate, lines[min(start+size, len(lines)):]...)
		ok, err := predicate(bytes.Join(candidate, nil))
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

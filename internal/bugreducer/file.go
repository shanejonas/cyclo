package bugreducer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// A checker can replace its input with a link or change its permissions.
// Replace the directory entry instead of truncating the previous candidate.
func writeCandidate(path string, source []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".candidate-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(source)
	err = errors.Join(writeErr, file.Close())
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

// Check the input type before reading: FIFOs can block forever and devices can
// produce unbounded data before any checker timeout takes effect.
func readInput(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input %q must be a regular file", path)
	}
	return os.ReadFile(path)
}

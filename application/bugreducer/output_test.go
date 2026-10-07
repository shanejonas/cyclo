package bugreducer

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckerOutputTailDoesNotDependOnWriteBoundaries(t *testing.T) {
	source := strings.Repeat("0123456789", previewLimit/5) + "final output"
	for _, chunkSize := range []int{1, 7, previewLimit - 1, previewLimit, previewLimit + 1, len(source)} {
		t.Run(fmt.Sprint(chunkSize), func(t *testing.T) {
			output := &checkerOutput{}
			for start := 0; start < len(source); start += chunkSize {
				end := min(start+chunkSize, len(source))
				data := []byte(source[start:end])
				if n, err := output.Write(data); n != len(data) || err != nil {
					t.Fatalf("Write = %d, %v", n, err)
				}
				if string(data) != source[start:end] {
					t.Fatal("Write changes the input")
				}
				want := source[max(end-previewLimit, 0):end]
				if output.snapshot() != want {
					t.Fatalf("incorrect tail after %d bytes", end)
				}
			}
			if n, err := output.Write(nil); n != 0 || err != nil {
				t.Fatalf("empty Write = %d, %v", n, err)
			}
			if output.snapshot() != source[len(source)-previewLimit:] {
				t.Fatal("empty Write changes the tail")
			}
		})
	}
}

package single

import "fmt"

// Entirely unique: no parallel structure anywhere near it.
func Frobnicator(input string, count int) (string, error) {
	if count < 0 {
		return "", fmt.Errorf("negative: %d", count)
	}
	out := input
	for i := 0; i < count; i++ {
		out = out + "!"
	}
	return out, nil
}

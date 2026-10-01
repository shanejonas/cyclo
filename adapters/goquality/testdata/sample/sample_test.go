package sample

import "testing"

func TestPure(t *testing.T) {
	if Pure(1) != 2 {
		t.Fatal("bad result")
	}
}

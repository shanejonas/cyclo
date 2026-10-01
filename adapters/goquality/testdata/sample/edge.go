package sample

func pointerResult() (*int, error) { return new(int), nil }

func boxResult() *Box { return new(Box) }

func TupleAndTemporary(values []int) {
	alias, err := pointerResult()
	_ = err
	*alias = 1
	boxResult().Count++
	copy(values[1:], []int{1})
}

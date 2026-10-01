package sample

import "unsafe"

type Pointer = unsafe.Pointer

func Unsafe(p *int) {
	q := Pointer(p)
	_ = unsafe.Add(q, 0)
	_ = unsafe.Sizeof(p)
}

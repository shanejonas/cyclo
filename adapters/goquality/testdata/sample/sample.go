package sample

import (
	f "fmt"
	"io"
	"os/exec"
	"time"
)

var global int

type Box struct {
	Count  int
	Values []int
}

func Pure(n int) int { return n + 1 }

func (b *Box) Read() int { return b.Count }

func (b *Box) Bump() { b.Count++ }

func Writes(b *Box, p *int, data []int, m map[string]int, a [1]int) {
	local := 0
	local++
	local += 2
	b.Count++
	alias := p
	*alias = 1
	owned := new(int)
	*owned = 1
	data[0] = 1
	m["x"] = 1
	a[0] = 1
	delete(m, "x")
	f.Println(local)
	_ = exec.Command("true")
	_ = time.Now()
	global++
	{
		local := 1
		local++
	}
	closure := func() { local++ }
	_ = closure
}

func Calls(b *Box) {
	b.Read()
	b.Bump()
}

func Dynamic(r io.Reader, callback func()) {
	r.Read(nil)
	callback()
}

func Aliases(p *int, flag bool, values []int) {
	local := 0
	alias := &local
	if flag {
		alias = p
	}
	*alias = 2
	buffer := make([]int, 2)
	buffer[0] = 1
	copy(values, buffer)
	values = append(values, 3)
}

// cyclo-allow(fn_params): stable public interface
// Suppressed keeps its API.
func Suppressed(a, b, c, d, e int) int { return a + b + c + d + e }

var Handler = func(p *int) { *p = 1 }

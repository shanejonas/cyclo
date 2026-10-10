package patterns

import "math/bits"

// ccPairs stores each unordered pair once. Rows have separate words, so
// workers that own distinct rows can write without locks or shared words.
type ccPairs struct {
	words []uint64
	width int
	size  int
}

func newCCPairs(size int) *ccPairs {
	width := (size + 63) / 64
	return &ccPairs{words: make([]uint64, size*width), width: width, size: size}
}

func (p *ccPairs) add(i, j int) {
	p.words[i*p.width+j/64] |= uint64(1) << (j % 64)
}

func (p *ccPairs) has(i, j int) bool {
	if i > j {
		i, j = j, i
	}
	return p.words[i*p.width+j/64]&(uint64(1)<<(j%64)) != 0
}

func (p *ccPairs) members() []int {
	seen := make([]bool, p.size)
	for offset, word := range p.words {
		if word == 0 {
			continue
		}
		seen[offset/p.width] = true
		for word != 0 {
			seen[(offset%p.width)*64+bits.TrailingZeros64(word)] = true
			word &= word - 1
		}
	}
	var out []int
	for i, present := range seen {
		if present {
			out = append(out, i)
		}
	}
	return out
}

// astShape uses one shared coordinate for each AST node kind.
type astShape struct {
	counts []int
	total  int
}

func ccASTShapes(ids []string, types map[string]map[string]int) []astShape {
	columns := map[string]int{}
	for _, id := range ids {
		for kind := range types[id] {
			if _, ok := columns[kind]; !ok {
				columns[kind] = len(columns)
			}
		}
	}
	out := make([]astShape, len(ids))
	for i, id := range ids {
		out[i] = denseASTShape(types[id], columns)
	}
	return out
}

func denseASTShape(types map[string]int, columns map[string]int) astShape {
	if len(types) == 0 {
		return astShape{}
	}
	shape := astShape{counts: make([]int, len(columns))}
	for kind, count := range types {
		shape.counts[columns[kind]] = count
		shape.total += count
	}
	return shape
}

func (a astShape) bypass(b astShape) bool {
	if len(a.counts) == 0 || len(b.counts) == 0 {
		return false
	}
	// Intersection cannot exceed the smaller total; union is at least
	// the larger. Reject only pairs that cannot reach the same threshold.
	largest := max(a.total, b.total)
	if largest == 0 {
		return true
	}
	if float64(min(a.total, b.total))/float64(largest) < astBypassThreshold {
		return false
	}
	inter := 0
	for i, count := range a.counts {
		inter += min(count, b.counts[i])
	}
	union := a.total + b.total - inter
	return float64(inter)/float64(union) >= astBypassThreshold
}

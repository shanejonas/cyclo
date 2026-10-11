package gopatterns

import "golang.org/x/tools/go/cfg"

// Immediate dominators use reverse postorder and predecessor intersection.
// All arrays and predecessor lists are temporary and linear in CFG size.
type executionDominators struct {
	order        []int
	rank, idom   []int
	predecessors [][]int
}

func newExecutionDominators(graph *cfg.CFG) *executionDominators {
	d := &executionDominators{rank: make([]int, len(graph.Blocks)), idom: make([]int, len(graph.Blocks)), predecessors: make([][]int, len(graph.Blocks))}
	seen := make([]bool, len(graph.Blocks))
	d.postorder(graph.Blocks[0], seen)
	for i := range d.idom {
		d.idom[i] = -1
	}
	for i := range len(d.order) / 2 {
		j := len(d.order) - 1 - i
		d.order[i], d.order[j] = d.order[j], d.order[i]
	}
	for i, index := range d.order {
		d.rank[index] = i
	}
	d.predecessors = executionPredecessors(graph)
	d.idom[0] = 0
	for d.update() {
	}
	return d
}

func (d *executionDominators) postorder(block *cfg.Block, seen []bool) {
	if seen[block.Index] {
		return
	}
	seen[block.Index] = true
	for _, next := range block.Succs {
		d.postorder(next, seen)
	}
	d.order = append(d.order, int(block.Index))
}

func (d *executionDominators) update() bool {
	changed := false
	for _, index := range d.order[1:] {
		parent := d.parent(index)
		if parent == d.idom[index] {
			continue
		}
		d.idom[index] = parent
		changed = true
	}
	return changed
}

func (d *executionDominators) parent(index int) int {
	parent := -1
	for _, previous := range d.predecessors[index] {
		if d.idom[previous] < 0 {
			continue
		}
		if parent < 0 {
			parent = previous
			continue
		}
		parent = d.intersect(parent, previous)
	}
	return parent
}

func (d *executionDominators) intersect(a, b int) int {
	for a != b {
		if d.rank[a] > d.rank[b] {
			a = d.idom[a]
			continue
		}
		b = d.idom[b]
	}
	return a
}

func executionPredecessors(graph *cfg.CFG) [][]int {
	predecessors := make([][]int, len(graph.Blocks))
	for _, block := range graph.Blocks {
		for _, next := range block.Succs {
			predecessors[next.Index] = append(predecessors[next.Index], int(block.Index))
		}
	}
	return predecessors
}

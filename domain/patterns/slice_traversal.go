package patterns

// returnTraversal owns the adjacency and BFS scratch for all return seeds
// in one function. blocked nodes cannot be entered; a seed remains included.
type returnTraversal struct {
	backward [][]int
	seen     []bool
	queue    []int
}

func newReturnTraversal(pdg *Pdg, thin bool) *returnTraversal {
	backward := make([][]int, len(pdg.Nodes))
	for _, e := range pdg.Edges {
		if thin && (e.Kind != Data || !isProducer(pdg.Nodes[e.From])) {
			continue
		}
		backward[e.To] = append(backward[e.To], e.From)
	}
	return &returnTraversal{backward: backward, seen: make([]bool, len(pdg.Nodes)), queue: make([]int, 0, len(pdg.Nodes))}
}

func (t *returnTraversal) size(seed int, blocked []bool) int {
	clear(t.seen)
	t.seen[seed] = true
	t.queue = append(t.queue[:0], seed)
	for head := 0; head < len(t.queue); head++ {
		for _, prev := range t.backward[t.queue[head]] {
			if t.seen[prev] || blocked[prev] {
				continue
			}
			t.seen[prev] = true
			t.queue = append(t.queue, prev)
		}
	}
	return len(t.queue)
}

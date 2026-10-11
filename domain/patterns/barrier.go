package patterns

// Barrier Slicing (Krinke, SQJ 2004):
// Declare parts of the dependence graph off-limits and the slice can't
// transitively pass through them. Unifies two ad-hoc practices: removing
// dependences from the graph, and dicing (removing known-good code from a
// slice).
//
// Use cases: mark code known bug-free as a barrier during debugging;
// ignore error-handling code when you only care about normal execution.
// For cyclo: the agent marks reviewed code as a barrier and the fault area
// shrinks each iteration.

// BarrierSlice computes the backward slice from seed within a PDG, without
// traversing through barrier nodes. barriers holds the PDG node indices
// that are off-limits: predecessors are not explored past them, so the
// slice can't transitively pass through a barrier.
// Returns the node indices in the slice.
func BarrierSlice(pdg *MiningGraph, seed int, barriers map[int]bool) []int {
	slice := barrierSliceSet(pdg, seed, barriers)
	var out []int
	for n := range slice {
		out = append(out, n)
	}
	return out
}

// barrierSliceSet returns the set of nodes in the barrier slice.
func barrierSliceSet(pdg *MiningGraph, seed int, barriers map[int]bool) map[int]bool {
	visited := map[int]bool{seed: true}
	queue := []int{seed}
	// Build reverse adjacency list.
	radj := map[int][]int{}
	for _, e := range pdg.Edges {
		radj[e.To] = append(radj[e.To], e.From)
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, prev := range radj[n] {
			if visited[prev] || barriers[prev] {
				continue
			}
			visited[prev] = true
			queue = append(queue, prev)
		}
	}
	return visited
}

// BarrierSliceLines is a convenience wrapper that returns source lines
// instead of node indices.
func BarrierSliceLines(pdg *MiningGraph, seed int, barriers map[int]bool) []int {
	nodes := BarrierSlice(pdg, seed, barriers)
	lines := make([]int, len(nodes))
	for i, n := range nodes {
		lines[i] = pdg.Nodes[n].Line
	}
	return lines
}

// LinesToBarriers maps source lines to PDG node indices, returning the
// barrier set for BarrierSlice. All nodes on the given lines become
// barriers.
func LinesToBarriers(pdg *MiningGraph, lines []int) map[int]bool {
	lineSet := map[int]bool{}
	for _, l := range lines {
		lineSet[l] = true
	}
	barriers := map[int]bool{}
	for i, n := range pdg.Nodes {
		if lineSet[n.Line] {
			barriers[i] = true
		}
	}
	return barriers
}

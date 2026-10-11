package patterns

// Thin Slicing (Sridharan, Fink, Bodik, PLDI 2007):
// Traditional slices are too big for humans because they include everything
// that MIGHT affect a value, including control dependences and heap
// plumbing. A thin slice includes only PRODUCER statements — the chain of
// assignments that compute and copy the value to the seed.
//
// For cyclo: when flagging side_effect_density or explaining a candidate,
// show the thin slice (5 lines) instead of the full dependence cone (50).
// This is the "attention" part of "AI's guide for attention."

// ThinSlice returns the producer chain for a value at a PDG node.
// Follows only Data edges backward, skipping control dependences.
// Returns node indices, ordered from seed backward to producers.
func ThinSlice(pdg *MiningGraph, seed int) []int {
	radj := buildProducerGraph(pdg)
	visited := map[int]bool{seed: true}
	queue := []int{seed}
	var result []int
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		result = append(result, n)
		queue = visitProducers(queue, visited, radj[n], pdg)
	}
	return result
}

// buildProducerGraph builds reverse data-edge adjacency (producers only).
func buildProducerGraph(pdg *MiningGraph) map[int][]int {
	radj := map[int][]int{}
	for _, e := range pdg.Edges {
		if e.Kind == Data {
			radj[e.To] = append(radj[e.To], e.From)
		}
	}
	return radj
}

// visitProducers adds unvisited producer nodes to the queue.
func visitProducers(queue []int, visited map[int]bool, prods []int, pdg *MiningGraph) []int {
	for _, prod := range prods {
		if !visited[prod] && isProducer(pdg.Nodes[prod]) {
			visited[prod] = true
			queue = append(queue, prod)
		}
	}
	return queue
}

// isProducer reports whether a node is a value producer (not control flow
// or heap plumbing). For now: exclude control nodes and address computations.
// This is a heuristic; the paper has a more precise definition.
func isProducer(n PdgNode) bool {
	// Skip control-flow nodes.
	switch n.Kind {
	case "Ctrl", "Branch", "Loop":
		return false
	}
	// Skip address-of and dereference plumbing (heap manipulation).
	// The Detail field may indicate this; for now, keep it simple.
	return true
}

// ThinSliceLines returns source lines for the thin slice.
func ThinSliceLines(pdg *MiningGraph, seed int) []int {
	nodes := ThinSlice(pdg, seed)
	lines := make([]int, len(nodes))
	for i, n := range nodes {
		lines[i] = pdg.Nodes[n].Line
	}
	return lines
}

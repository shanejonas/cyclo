package patterns

// Chopping (Jackson & Rollins, CMU-CS-94-169, 1994):
// A chop between a source criterion and a sink criterion is the set of
// statements that transmit effects from source to sink — the intersection
// of the forward slice of the source with the backward slice of the sink.
//
// Answers "how does THIS input reach THAT output?" rather than "what
// affects X?" For cyclo: given changed lines (sources) and a sink
// (DB writes, network calls, error returns), show exactly the statements
// on the path between them.

// Chop computes the statements on the dependence path from source to sink
// within a PDG. source and sink are PDG node indices.
// Returns the node indices in the chop, sorted.
func Chop(pdg *MiningGraph, source, sink int) []int {
	fwd := forwardSlice(pdg, source)
	bwd := backwardSlice(pdg, sink)
	// Intersection.
	var chop []int
	for n := range fwd {
		if bwd[n] {
			chop = append(chop, n)
		}
	}
	return chop
}

// forwardSlice returns all nodes reachable from start via dependence edges.
func forwardSlice(pdg *MiningGraph, start int) map[int]bool {
	visited := map[int]bool{start: true}
	queue := []int{start}
	// Build adjacency list.
	adj := map[int][]int{}
	for _, e := range pdg.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, next := range adj[n] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	return visited
}

// backwardSlice returns all nodes that can reach target via dependence edges.
func backwardSlice(pdg *MiningGraph, target int) map[int]bool {
	visited := map[int]bool{target: true}
	queue := []int{target}
	// Build reverse adjacency list.
	radj := map[int][]int{}
	for _, e := range pdg.Edges {
		radj[e.To] = append(radj[e.To], e.From)
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, prev := range radj[n] {
			if !visited[prev] {
				visited[prev] = true
				queue = append(queue, prev)
			}
		}
	}
	return visited
}

// ChopLines is a convenience wrapper that returns source lines instead of
// node indices.
func ChopLines(pdg *MiningGraph, source, sink int) []int {
	nodes := Chop(pdg, source, sink)
	lines := make([]int, len(nodes))
	for i, n := range nodes {
		lines[i] = pdg.Nodes[n].Line
	}
	return lines
}

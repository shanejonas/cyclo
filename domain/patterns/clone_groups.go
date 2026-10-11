package patterns

import "sort"

// find is union-find lookup with path compression.
func find(parent map[string]string, x string) string {
	for parent[x] != x {
		parent[x] = parent[parent[x]]
		x = parent[x]
	}
	return x
}

// union merges the sets containing a and b.
func union(parent map[string]string, a, b string) {
	ra, rb := find(parent, a), find(parent, b)
	if ra != rb {
		parent[ra] = rb
	}
}

// groupsOfTwoOrMore collects the union-find sets of size >= 2, sorted
// deterministically: members sorted within each group, groups sorted by
// first member.
func groupsOfTwoOrMore(parent map[string]string) [][]string {
	byRoot := map[string][]string{}
	for id := range parent {
		root := find(parent, id)
		byRoot[root] = append(byRoot[root], id)
	}
	var out [][]string
	for _, group := range byRoot {
		if len(group) >= 2 {
			sort.Strings(group)
			out = append(out, group)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

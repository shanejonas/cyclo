package patterns

import (
	"strings"

	"github.com/shanejonas/cyclo/domain/pdg"
)

// Keys contain exactly the fields used by label(), not identity or effects.
// Values use stable content hashes, never extraction-local table IDs.
type baseLabelKey struct {
	kind         NodeKind
	shape, scope string
}
type baseLabelCache struct{ hashes map[baseLabelKey]uint64 }

func baseKey(n PdgNode) baseLabelKey {
	key := baseLabelKey{kind: n.Kind}
	switch n.Kind {
	case Call:
		key.shape = n.SigClass
		key.scope = callScope(n.CalleeID)
	case Lit:
		key.shape = n.LitKind
	case Op:
		key.shape, _, _ = strings.Cut(n.Detail, ":")
	case Param:
		key.shape = n.TyClass
	}
	return key
}

func sharedBaseLabels(facts []*FuncFacts) *baseLabelCache {
	cache := &baseLabelCache{hashes: map[baseLabelKey]uint64{}}
	seen := map[*pdg.Tables]bool{}
	for _, f := range facts {
		if f == nil || f.Pdg == nil {
			continue
		}
		cache.tables(f.Pdg.Tables, seen)
	}
	return cache
}

func (c *baseLabelCache) tables(t *pdg.Tables, seen map[*pdg.Tables]bool) {
	if t == nil || seen[t] {
		return
	}
	seen[t] = true
	for _, l := range t.MatchLabels {
		c.add(miningLabel(t, l, 0))
	}
}

func (c *baseLabelCache) add(n PdgNode) {
	key := baseKey(n)
	if _, ok := c.hashes[key]; ok {
		return
	}
	c.hashes[key] = fnv1a([]byte(label(n)))
}

func (c *baseLabelCache) attach(g *MiningGraph) {
	if g == nil {
		return
	}
	g.baseLabels = c
}

func (c *baseLabelCache) hash(n PdgNode) uint64 {
	if c == nil {
		return fnv1a([]byte(label(n)))
	}
	if hash, ok := c.hashes[baseKey(n)]; ok {
		return hash
	}
	// Normalization can introduce labels absent from the extracted table.
	// The published map stays read-only, so concurrent readers remain safe.
	return fnv1a([]byte(label(n)))
}

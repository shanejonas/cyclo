package patterns

import (
	"path/filepath"
	"testing"
)

func cacheTestPdg() *Pdg {
	return &Pdg{
		Nodes: []PdgNode{
			{Kind: Branch, Line: 10},
			{Kind: Call, CalleeID: "fmt.Println", SigClass: "fn(string) -> ()", Line: 11},
			{Kind: Op, Detail: "arith:+", Line: 12},
		},
		Edges: []PdgEdge{
			{From: 0, To: 1, Kind: Ctrl, ArgPos: 0},
			{From: 0, To: 2, Kind: Ctrl, ArgPos: 1},
			{From: 2, To: 1, Kind: Data, ArgPos: 0},
		},
	}
}

func TestWlCacheHit(t *testing.T) {
	c := NewWlCache()
	pdg := cacheTestPdg()
	w := NewWl(pdg)
	c.Put(pdg, w)

	got, ok := c.Get(pdg)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if SimilarityMilli(w, got) != 1000 {
		t.Errorf("cached WL similarity to original = %d, want 1000", SimilarityMilli(w, got))
	}
	if hits, misses := c.Stats(); hits != 1 || misses != 0 {
		t.Errorf("stats = %d/%d, want 1/0", hits, misses)
	}
}

func TestWlCacheMissOnChange(t *testing.T) {
	c := NewWlCache()
	pdg := cacheTestPdg()
	c.Put(pdg, NewWl(pdg))

	changed := cacheTestPdg()
	changed.Nodes = append(changed.Nodes, PdgNode{Kind: Op, Detail: "cmp:==", Line: 13})
	if _, ok := c.Get(changed); ok {
		t.Error("expected cache miss for changed PDG")
	}
	if _, misses := c.Stats(); misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
}

func TestWlCacheIgnoresLineNumbers(t *testing.T) {
	c := NewWlCache()
	pdg := cacheTestPdg()
	c.Put(pdg, NewWl(pdg))

	moved := cacheTestPdg()
	for i := range moved.Nodes {
		moved.Nodes[i].Line += 100
	}
	if _, ok := c.Get(moved); !ok {
		t.Error("line-number shift should not invalidate the cache entry")
	}
}

func TestWlCacheSaveLoad(t *testing.T) {
	c := NewWlCache()
	pdg := cacheTestPdg()
	w := NewWl(pdg)
	c.Put(pdg, w)

	path := filepath.Join(t.TempDir(), "wlcache.json")
	if err := c.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	c2 := NewWlCache()
	if err := c2.Load(path); err != nil {
		t.Fatalf("load: %v", err)
	}
	got, ok := c2.Get(pdg)
	if !ok {
		t.Fatal("expected hit after load")
	}
	if SimilarityMilli(w, got) != 1000 {
		t.Errorf("reloaded WL similarity = %d, want 1000", SimilarityMilli(w, got))
	}
	// Weighted similarity also works from a reloaded entry (lazy projections
	// rebuild from the PDG).
	if SimilarityWeighted(w, got) != 1000 {
		t.Errorf("reloaded weighted similarity = %d, want 1000", SimilarityWeighted(w, got))
	}
}

func TestWlCacheLoadMissingFile(t *testing.T) {
	c := NewWlCache()
	if err := c.Load(filepath.Join(t.TempDir(), "nope.json")); err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
}

func TestClusterPdgsCachedMatchesUncached(t *testing.T) {
	graphs := windowGraphs()
	params := clusterParams()

	plain := ClusterPdgs(graphs, params)

	cache := NewWlCache()
	cached := ClusterPdgsCached(graphs, params, cache)
	if len(cached) != len(plain) {
		t.Fatalf("cached clusters = %d, want %d", len(cached), len(plain))
	}
	hits, misses := cache.Stats()
	if hits != 0 || misses != len(graphs) {
		t.Errorf("cold stats = %d/%d, want 0/%d", hits, misses, len(graphs))
	}

	// Second run: all hits, same result.
	cached2 := ClusterPdgsCached(graphs, params, cache)
	if len(cached2) != len(plain) {
		t.Fatalf("warm cached clusters = %d, want %d", len(cached2), len(plain))
	}
	hits, _ = cache.Stats()
	if hits != len(graphs) {
		t.Errorf("warm hits = %d, want %d", hits, len(graphs))
	}
	// Members must match between cached and uncached.
	for i := range plain {
		if len(cached2[i].Members) != len(plain[i].Members) {
			t.Errorf("cluster %d members differ", i)
		}
		for j := range plain[i].Members {
			if cached2[i].Members[j] != plain[i].Members[j] {
				t.Errorf("cluster %d member %d differs", i, j)
			}
		}
	}
}

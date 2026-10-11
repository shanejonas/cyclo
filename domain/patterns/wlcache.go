package patterns

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// WlCache memoizes WL refinements keyed by PDG content hash, so repeated
// miner runs (e.g. patterns --changed on a big repo) only recompute the
// functions whose bodies changed. The key hashes the canonical PDG without
// line numbers: moving a function does not invalidate its entry.
type WlCache struct {
	entries map[string]wlCacheEntry
	hits    int
	misses  int
}

// wlCacheVersion versions the cache format AND the WL computation. Bump it
// whenever label(), the kernel, or canonicalization changes, so stale
// entries from an older miner are never reused.
const wlCacheVersion = 3

// wlCacheFile is the JSON-serialized cache with a version header.
type wlCacheFile struct {
	Version int                     `json:"version"`
	Entries map[string]wlCacheEntry `json:"entries"`
}

// wlCacheEntry is the JSON-serializable WL state. Histograms are rebuilt
// from rounds on load; the graph is rebuilt from the PDG on cache hit.
type wlCacheEntry struct {
	Rounds   [][]uint64 `json:"rounds"`
	Diameter int        `json:"diameter"`
	Calls    int        `json:"calls"`
	CharVec  []float64  `json:"charVec"`
}

// NewWlCache returns an empty cache.
func NewWlCache() *WlCache {
	return &WlCache{entries: map[string]wlCacheEntry{}}
}

// pdgKey hashes the PDG's semantic content (kinds, labels, edges) with FNV-1a.
// Line numbers are excluded: shifting a function in its file is not a change.
func pdgKey(pdg *MiningGraph) string {
	h := fnvOffset64
	write := func(s string) {
		for i := 0; i < len(s); i++ {
			h ^= uint64(s[i])
			h *= fnvPrime64
		}
		h ^= 0xff
		h *= fnvPrime64
	}
	for _, n := range pdg.Nodes {
		write(string(n.Kind))
		write(n.TyClass)
		write(n.SigClass)
		write(n.CalleeID)
		write(n.LitKind)
		write(n.Detail)
	}
	write("|")
	for _, e := range pdg.Edges {
		write(strconv.Itoa(e.From))
		write(strconv.Itoa(e.To))
		write(string(e.Kind))
		write(strconv.Itoa(e.ArgPos))
	}
	return strconv.FormatUint(h, 16)
}

// Get returns the cached WL refinement for pdg, rebuilding the graph from
// the PDG (cheap) while reusing the rounds, histograms inputs, diameter,
// call count, and characteristic vector.
func (c *WlCache) Get(pdg *MiningGraph) (*Wl, bool) {
	if c == nil {
		return nil, false
	}
	e, ok := c.entries[pdgKey(pdg)]
	if !ok {
		c.misses++
		return nil, false
	}
	c.hits++
	g := buildGraph(pdg)
	return &Wl{
		graph:     g,
		rounds:    e.Rounds,
		hists:     histograms(e.Rounds),
		calls:     e.Calls,
		diameter:  e.Diameter,
		charVec:   e.CharVec,
		pdg:       pdg,
		nodeCount: len(g.labels),
	}, true
}

// Put stores w's refinement under pdg's content key.
func (c *WlCache) Put(pdg *MiningGraph, w *Wl) {
	if c == nil {
		return
	}
	c.entries[pdgKey(pdg)] = wlCacheEntry{
		Rounds:   w.rounds,
		Diameter: w.diameter,
		Calls:    w.calls,
		CharVec:  w.charVec,
	}
}

// Stats returns cache hits and misses since creation or the last Reset.
func (c *WlCache) Stats() (hits, misses int) {
	if c == nil {
		return 0, 0
	}
	return c.hits, c.misses
}

// Reset clears entries and counters.
func (c *WlCache) Reset() {
	if c == nil {
		return
	}
	c.entries = map[string]wlCacheEntry{}
	c.hits, c.misses = 0, 0
}

// Save persists the cache to path as JSON, creating parent directories.
func (c *WlCache) Save(path string) error {
	if c == nil {
		return nil
	}
	data, err := json.Marshal(wlCacheFile{Version: wlCacheVersion, Entries: c.entries})
	if err != nil {
		return fmt.Errorf("marshal wl cache: %w", err)
	}
	if err := ensureCacheDir(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write wl cache %s: %w", path, err)
	}
	return nil
}

// ensureCacheDir creates the parent directory of path, unless path is bare.
func ensureCacheDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir wl cache dir %s: %w", dir, err)
	}
	return nil
}

// Load reads a cache file written by Save. A missing file is not an error:
// the first run simply starts cold. Files with a different version are
// ignored (stale). Corrupt files are reported.
func (c *WlCache) Load(path string) error {
	if c == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read wl cache %s: %w", path, err)
	}
	var file wlCacheFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse wl cache %s: %w", path, err)
	}
	if file.Version != wlCacheVersion {
		return nil
	}
	c.entries = file.Entries
	return nil
}

// cacheKeysForTest exposes entry keys for white-box tests.
func (c *WlCache) cacheKeysForTest() []string {
	keys := make([]string, 0, len(c.entries))
	for k := range c.entries {
		keys = append(keys, k)
	}
	return keys
}

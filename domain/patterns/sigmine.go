// Package patterns: sigmine is Layer 1 of the patterns miner — it groups
// methods of different types by normalized signature. A blocking key for
// later layers, not a finding by itself. Ported from rstyle-core's
// sigmine.rs.
package patterns

import (
	"math"
	"sort"
	"strings"
)

// Member is one method in a signature group.
type Member struct {
	Path   string
	Line   int
	ID     string
	Name   string
	SelfTy string
}

// SigGroup is a set of inherent methods sharing one normalized signature,
// spanning at least two distinct self types.
type SigGroup struct {
	// Key is the normalized signature (FuncFacts.SigKey).
	Key string
	// Members are inherent methods, one per site; at least two distinct SelfTy.
	Members []Member
	// Traited are trait-impl methods with the same signature: counted, never proposed.
	Traited []Member
	// SelfTypes is the number of distinct self types among Members.
	SelfTypes int
	// SameName is true when two or more distinct types define a method with
	// the same short name.
	SameName bool
	// SpecificityMilli is 1000 * ln(total / group size): rare signatures score
	// high, getter-like common ones near zero.
	SpecificityMilli uint32
	// ScoreMilli is specificity plus up to 50%, scaled by the share of types
	// that share a name.
	ScoreMilli uint32
}

// shortName mirrors rstyle's shape::short: the method name after its final
// scope separator (Go's '.' instead of Rust's '::').
func shortName(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func memberOf(f *FuncFacts) Member {
	return Member{
		Path:   f.Path,
		Line:   f.Line,
		ID:     f.ID,
		Name:   f.Name,
		SelfTy: f.SelfTy,
	}
}

// sortedMembers sorts and dedupes by function id, so input order never matters.
func sortedMembers(members []Member) []Member {
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	out := members[:0]
	for _, m := range members {
		if len(out) == 0 || out[len(out)-1].ID != m.ID {
			out = append(out, m)
		}
	}
	return out
}

func specificityMilli(total, size int) uint32 {
	ratio := float64(total) / math.Max(1, float64(size))
	milli := math.Round(math.Log(ratio) * 1000)
	if milli < 0 {
		return 0
	}
	return uint32(milli)
}

// sharedNameTypes is the largest number of distinct self types that share
// one short method name (0 or 1: none).
func sharedNameTypes(members []Member) int {
	owners := map[string]map[string]bool{}
	for _, m := range members {
		key := shortName(m.Name)
		if owners[key] == nil {
			owners[key] = map[string]bool{}
		}
		owners[key][m.SelfTy] = true
	}
	best := 0
	for _, tys := range owners {
		if len(tys) > best {
			best = len(tys)
		}
	}
	return best
}

type bucketEntry struct {
	member  Member
	traited bool
}

// build partitions a bucket into inherent/traited, requires at least two
// distinct self types, and scores the group.
func build(key string, entries []bucketEntry, total int) *SigGroup {
	var inherent, traited []Member
	for _, e := range entries {
		if e.traited {
			traited = append(traited, e.member)
		} else {
			inherent = append(inherent, e.member)
		}
	}
	members := sortedMembers(inherent)
	traited = sortedMembers(traited)
	types := map[string]bool{}
	for _, m := range members {
		types[m.SelfTy] = true
	}
	if len(types) < 2 {
		return nil
	}
	specificity := specificityMilli(total, len(members)+len(traited))
	shared := sharedNameTypes(members)
	sameName := shared >= 2
	// The boost scales with the share of types that actually share the name.
	var boost uint32
	if sameName {
		boost = uint32(uint64(specificity) / 2 * uint64(shared) / uint64(len(types)))
	}
	return &SigGroup{
		Key:              key,
		Members:          members,
		Traited:          traited,
		SelfTypes:        len(types),
		SameName:         sameName,
		SpecificityMilli: specificity,
		ScoreMilli:       specificity + boost,
	}
}

// buckets collects methods by normalized signature key, skipping nil facts,
// free functions (no SelfTy), and unnormalized functions (no SigKey).
func buckets(facts []*FuncFacts) (map[string][]bucketEntry, int) {
	buckets := map[string][]bucketEntry{}
	total := 0
	for _, f := range facts {
		if f == nil || f.SigKey == "" || f.SelfTy == "" {
			continue
		}
		buckets[f.SigKey] = append(buckets[f.SigKey], bucketEntry{memberOf(f), f.Implements})
		total++
	}
	return buckets, total
}

func mineGroups(buckets map[string][]bucketEntry, total int) []SigGroup {
	groups := []SigGroup{}
	for key, entries := range buckets {
		if g := build(key, entries, total); g != nil {
			groups = append(groups, *g)
		}
	}
	return groups
}

// sortGroups orders groups best-first (score, then key) so output is
// independent of input order.
func sortGroups(groups []SigGroup) {
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].ScoreMilli != groups[j].ScoreMilli {
			return groups[i].ScoreMilli > groups[j].ScoreMilli
		}
		return groups[i].Key < groups[j].Key
	})
}

// MineGroups groups methods of ADTs by normalized signature key, returning the
// groups with two or more distinct self types, best first (score, then key)
// so output is independent of input order. Methods without a SigKey or with
// no receiver (free functions) are ignored, as are nil facts.
//
// (Renamed from Mine: candidates.go's layer-2 main entry takes the name Mine
// per its contract `func Mine(facts, groups, params) Mined`, and Go has no
// overloading.)
func MineGroups(facts []*FuncFacts) []SigGroup {
	buckets, total := buckets(facts)
	groups := mineGroups(buckets, total)
	sortGroups(groups)
	return groups
}

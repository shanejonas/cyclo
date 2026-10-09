package patterns

import "fmt"

// Slicing-based pattern kinds: barrier_slice, thin_slice, and chop.
//
// These are detection-only attention guides, not bug finders. They surface
// the dependence structure of a function so an LLM or human can see what
// affects a return value, how data flows, and which statements lie on the
// input-to-output path. Each fires only when the slice/chop is large enough
// to be non-obvious; trivial functions are skipped.

// Slice thresholds: the minimum slice/chop size (in PDG nodes) for a
// finding. Below this, the dependence structure is obvious from reading
// the function. These are heuristics, not paper thresholds.
const (
	minBarrierSliceNodes = 20
	minThinSliceNodes    = 10
	minChopNodes         = 20
)

// BarrierSliceFinding is a function whose return-value dependence cone
// (excluding error handling) is large enough to be worth surfacing.
type BarrierSliceFinding struct {
	FuncID    string
	SliceSize int
	SeedLine  int
}

// ThinSliceFinding is a function with a long producer chain feeding a
// return value.
type ThinSliceFinding struct {
	FuncID    string
	SliceSize int
	SeedLine  int
}

// ChopFinding is a function where the input-to-output dependence path
// covers many statements.
type ChopFinding struct {
	FuncID   string
	ChopSize int
}

// FindBarrierSlices computes the barrier slice from each return statement,
// using Try nodes (the `if err != nil { return err }` idiom) as barriers.
// This shows the happy-path dependencies of each return, excluding error
// handling. Flags functions where any return's slice is large.
func FindBarrierSlices(facts []*FuncFacts) []BarrierSliceFinding {
	var out []BarrierSliceFinding
	for _, f := range facts {
		if f == nil || f.Pdg == nil {
			continue
		}
		out = append(out, barrierSlicesForFunc(f)...)
	}
	return out
}

// barrierSlicesForFunc computes the barrier slice from each return statement
// in one function, flagging returns whose slice (excluding error handling)
// is large.
func barrierSlicesForFunc(f *FuncFacts) []BarrierSliceFinding {
	var out []BarrierSliceFinding
	barriers := tryBarriers(f.Pdg)
	for i, n := range f.Pdg.Nodes {
		if n.Kind != Return {
			continue
		}
		slice := BarrierSlice(f.Pdg, i, barriers)
		if len(slice) >= minBarrierSliceNodes {
			out = append(out, BarrierSliceFinding{
				FuncID:    f.ID,
				SliceSize: len(slice),
				SeedLine:  n.Line,
			})
		}
	}
	return out
}

// tryBarriers returns the set of Try node indices (error-handling idiom
// nodes) to use as barriers.
func tryBarriers(pdg *Pdg) map[int]bool {
	barriers := map[int]bool{}
	for i, n := range pdg.Nodes {
		if n.Kind == Try {
			barriers[i] = true
		}
	}
	return barriers
}

// FindThinSlices computes the thin (producer-only) slice from each return
// statement. Flags functions where any return's producer chain is long.
func FindThinSlices(facts []*FuncFacts) []ThinSliceFinding {
	var out []ThinSliceFinding
	for _, f := range facts {
		if f == nil || f.Pdg == nil {
			continue
		}
		out = append(out, thinSlicesForFunc(f)...)
	}
	return out
}

// thinSlicesForFunc computes the thin (producer-only) slice from each return
// statement in one function, flagging returns with a long producer chain.
func thinSlicesForFunc(f *FuncFacts) []ThinSliceFinding {
	var out []ThinSliceFinding
	for i, n := range f.Pdg.Nodes {
		if n.Kind != Return {
			continue
		}
		slice := ThinSlice(f.Pdg, i)
		if len(slice) >= minThinSliceNodes {
			out = append(out, ThinSliceFinding{
				FuncID:    f.ID,
				SliceSize: len(slice),
				SeedLine:  n.Line,
			})
		}
	}
	return out
}

// FindChops computes the chop from function parameters to return statements
// (the statements on the input-to-output dependence path). Flags functions
// where the chop is large.
func FindChops(facts []*FuncFacts) []ChopFinding {
	var out []ChopFinding
	for _, f := range facts {
		if f == nil || f.Pdg == nil {
			continue
		}
		chop := paramToReturnChop(f.Pdg)
		if len(chop) >= minChopNodes {
			out = append(out, ChopFinding{
				FuncID:   f.ID,
				ChopSize: len(chop),
			})
		}
	}
	return out
}

// paramToReturnChop unions the chop from every param to every return.
func paramToReturnChop(pdg *Pdg) map[int]bool {
	params, returns := paramReturnIndices(pdg)
	union := map[int]bool{}
	for _, src := range params {
		for _, sink := range returns {
			for _, n := range Chop(pdg, src, sink) {
				union[n] = true
			}
		}
	}
	return union
}

// paramReturnIndices collects the node indices of parameters and returns.
func paramReturnIndices(pdg *Pdg) (params, returns []int) {
	for i, n := range pdg.Nodes {
		switch n.Kind {
		case Param:
			params = append(params, i)
		case Return:
			returns = append(returns, i)
		}
	}
	return params, returns
}

// BarrierSliceCandidates converts barrier slice findings to candidates.
// Detection-only: FixSpec is nil.
func BarrierSliceCandidates(findings []BarrierSliceFinding, facts []*FuncFacts) []Candidate {
	factByID := buildFactMap(facts)
	var out []Candidate
	for _, fl := range findings {
		f := factByID[fl.FuncID]
		if f == nil {
			continue
		}
		out = append(out, Candidate{
			Kind:       BarrierSliceKind,
			ScoreMilli: sliceScore(fl.SliceSize),
			Observation: fmt.Sprintf(
				"return at line %d depends on %d statements (excluding error handling)",
				fl.SeedLine, fl.SliceSize,
			),
			Inference:        "a large happy-path dependence cone is hard to reason about; consider simplifying",
			PossibleRefactor: "extract helpers for parts of the computation feeding this return",
			Sites: []Site{
				{Path: f.Path, Line: fl.SeedLine, Name: f.Name},
			},
		})
	}
	return out
}

// ThinSliceCandidates converts thin slice findings to candidates.
// Detection-only: FixSpec is nil.
func ThinSliceCandidates(findings []ThinSliceFinding, facts []*FuncFacts) []Candidate {
	factByID := buildFactMap(facts)
	var out []Candidate
	for _, fl := range findings {
		f := factByID[fl.FuncID]
		if f == nil {
			continue
		}
		out = append(out, Candidate{
			Kind:       ThinSliceKind,
			ScoreMilli: sliceScore(fl.SliceSize),
			Observation: fmt.Sprintf(
				"return at line %d has a %d-node producer chain",
				fl.SeedLine, fl.SliceSize,
			),
			Inference:        "a long producer chain means the value passes through many transformations",
			PossibleRefactor: "consider naming intermediate values or extracting the computation",
			Sites: []Site{
				{Path: f.Path, Line: fl.SeedLine, Name: f.Name},
			},
		})
	}
	return out
}

// ChopCandidates converts chop findings to candidates.
// Detection-only: FixSpec is nil.
func ChopCandidates(findings []ChopFinding, facts []*FuncFacts) []Candidate {
	factByID := buildFactMap(facts)
	var out []Candidate
	for _, fl := range findings {
		f := factByID[fl.FuncID]
		if f == nil {
			continue
		}
		out = append(out, Candidate{
			Kind:       ChopKind,
			ScoreMilli: sliceScore(fl.ChopSize),
			Observation: fmt.Sprintf(
				"%d statements lie on the input-to-output path",
				fl.ChopSize,
			),
			Inference:        "when most of the function is on the input-output path, the logic is tightly coupled",
			PossibleRefactor: "look for independent computations that can be extracted",
			Sites: []Site{
				{Path: f.Path, Line: f.Line, Name: f.Name},
			},
		})
	}
	return out
}

// sliceScore maps slice size to a milli-score. Larger slices score higher
// (more to pay attention to), capped at 800 — these are guides, not bugs.
func sliceScore(size int) uint32 {
	s := uint32(400 + size*10)
	if s > 800 {
		s = 800
	}
	return s
}

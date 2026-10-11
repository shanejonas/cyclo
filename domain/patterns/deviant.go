package patterns

import "fmt"

// Bugs as Deviant Behavior (Engler, Chen, Hallem, Chou, Chelf, SOSP 2001):
// Extract programmer beliefs from code, then find contradictions (MUST
// beliefs — always bugs) and statistical deviations (MAY beliefs).
//
// The key trick: you don't need to know the truth; a contradiction IS
// the bug. Engler commercialized this as Coverity (closed source); the
// open world never got the belief-inference engine.
//
// For cyclo: a `deviant_behavior` pattern. For each callee, tally beliefs
// across call sites (is the error checked? is the result nil-checked?).
// Flag sites where the behavior deviates from the overwhelming majority.
// This generalizes PR-Miner with the MUST/MAY distinction.

// minBeliefConfidence is the fraction of sites that must agree for a MAY
// belief to be actionable. Below this, it's just noise.
const minBeliefConfidence = 0.9

// minBeliefSupport is the minimum number of call sites for a belief.
const minBeliefSupport = 5

// CallBelief is a belief about how a callee's results are handled.
type CallBelief struct {
	// Callee is the called function name.
	Callee string
	// Belief describes the expected handling, e.g. "error checked".
	Belief string
	// Confidence is the fraction of sites that follow the belief.
	Confidence float64
	// Support is the number of call sites.
	Support int
}

// BeliefViolation is a call site that deviates from a belief.
type BeliefViolation struct {
	FuncID string
	Line   int
	Callee string
	Belief CallBelief
}

// ErrorCheckSite records whether a call's error was checked.
type ErrorCheckSite struct {
	FuncID  string
	Line    int
	Callee  string
	Checked bool // True if the error was checked (if err != nil, etc.)
}

// MineErrorBeliefs tallies error-checking behavior per callee.
// Returns beliefs where >=90% of sites check the error.
func MineErrorBeliefs(sites []ErrorCheckSite) []CallBelief {
	byCallee := groupByCallee(sites)
	var beliefs []CallBelief
	for callee, ss := range byCallee {
		if b, ok := beliefForCallee(callee, ss); ok {
			beliefs = append(beliefs, b)
		}
	}
	return beliefs
}

// groupByCallee groups sites by callee name.
func groupByCallee(sites []ErrorCheckSite) map[string][]ErrorCheckSite {
	out := map[string][]ErrorCheckSite{}
	for _, s := range sites {
		out[s.Callee] = append(out[s.Callee], s)
	}
	return out
}

// beliefForCallee creates a belief if the callee meets thresholds.
func beliefForCallee(callee string, ss []ErrorCheckSite) (CallBelief, bool) {
	if len(ss) < minBeliefSupport {
		return CallBelief{}, false
	}
	conf := checkedFraction(ss)
	if conf < minBeliefConfidence {
		return CallBelief{}, false
	}
	return CallBelief{
		Callee:     callee,
		Belief:     "error checked",
		Confidence: conf,
		Support:    len(ss),
	}, true
}

// checkedFraction returns the fraction of sites that check the error.
func checkedFraction(ss []ErrorCheckSite) float64 {
	checked := 0
	for _, s := range ss {
		if s.Checked {
			checked++
		}
	}
	return float64(checked) / float64(len(ss))
}

// FindBeliefViolations finds sites that deviate from mined beliefs.

// FindBeliefViolations finds sites that deviate from mined beliefs.
func FindBeliefViolations(sites []ErrorCheckSite, beliefs []CallBelief) []BeliefViolation {
	beliefByCallee := map[string]CallBelief{}
	for _, b := range beliefs {
		beliefByCallee[b.Callee] = b
	}
	var out []BeliefViolation
	for _, s := range sites {
		b, ok := beliefByCallee[s.Callee]
		if !ok || s.Checked {
			continue
		}
		// Site does not check the error, but the belief says it should.
		out = append(out, BeliefViolation{
			FuncID: s.FuncID,
			Line:   s.Line,
			Callee: s.Callee,
			Belief: b,
		})
	}
	return out
}

// DeviantBehaviorCandidates converts violations to pattern candidates.
func DeviantBehaviorCandidates(violations []BeliefViolation, facts []*MiningFacts) []Candidate {
	factByID := map[string]*MiningFacts{}
	for _, f := range facts {
		factByID[f.ID] = f
	}
	var out []Candidate
	for _, v := range violations {
		f := factByID[v.FuncID]
		if f == nil {
			continue
		}
		out = append(out, Candidate{
			Kind:        DeviantBehavior,
			ScoreMilli:  800, // High: deviant error handling is a bug.
			Observation: fmt.Sprintf("`%s` error not checked", v.Callee),
			Inference: fmt.Sprintf(
				"%.0f%% of %d call sites check this error; this one doesn't",
				v.Belief.Confidence*100, v.Belief.Support,
			),
			PossibleRefactor: "check the error: `if err != nil { ... }`",
			Sites: []Site{
				{Path: f.Path, Line: v.Line, Name: f.Name},
			},
		})
	}
	return out
}

# Research Papers Behind Cyclo

The static analysis research that cyclo implements, with the key insight
from each paper, what cyclo took from it, and how faithfully it was implemented.
These are the "buried gold" papers — old work that modern compute (and cyclo's
PDG extractor) unlocks.

## Summary

| Paper | Year | Venue | Pattern kind | Fidelity |
|---|---|---|---|---|
| Zou et al. (CCGraph) | 2020 | ASE | `ccgraph_clone` | Exact |
| Gabel, Jiang, Su | 2008 | ICSE | LSH layer in CCGraph | Adapted |
| Li et al. (CP-Miner) | 2004 | OSDI | `inconsistent_clone` | Adapted |
| Li & Zhou (PR-Miner) | 2005 | FSE | `mined_rule` | Adapted |
| Engler et al. | 2001 | SOSP | `deviant_behavior` | Inspired by |
| Thummalapenta & Xie (Alattin) | 2009 | ASE | `alattin_rule` | Adapted |
| Krinke (Barrier slicing) | 2004 | SQJ | library only | Adapted |
| Weimer & Necula | 2004 | OOPSLA | `obligation` | Adapted |
| Jackson & Rollins (Chopping) | 1994 | CMU | library only | Adapted |
| Sridharan, Fink & Bodik (Thin slicing) | 2007 | PLDI | library only | Adapted |
| Binkley & Harman (Dependence clusters) | 2015 | ICSME | `dependence_cluster` | Adapted |
| Horwitz (Semantic diff) | 1990 | PLDI | library only | Inspired by |
| Bulychev & Minea (Anti-unification) | 2008 | — | `parameterize` fixer | Exact |

**Fidelity levels:**
- **Exact:** Implemented as the paper describes, with paper's thresholds/parameters.
- **Adapted:** Core idea from the paper, modified for cyclo's architecture or Go.
- **Inspired by:** Paper motivated the approach; implementation differs significantly.

---

## Implemented

### CCGraph — Zou, Ban, Xue & Xu, ASE 2020

**Paper:** "CCGraph: a PDG-based code clone detector with approximate graph matching"
**DOI:** 10.1145/3324884.3416541

**The insight:** PDG-based clone detection was stuck on exact subgraph isomorphism
(NP-hard, slow, misses clones). CCGraph replaces exact matching with approximate
graph matching via the Weisfeiler-Lehman (WL) graph kernel, plus a two-stage
filtering strategy using characteristic vectors to prune candidates before the
expensive kernel computation.

**The pipeline (from the paper):**
1. **Characteristic vectors** (7-dimensional PDG numerical features) → cosine similarity ≥ **0.9**
2. **Jaro-Winkler** string similarity on function names → ratio ≥ **0.5**
3. **WL vectors** (512-dim) → locality-sensitive hashing for candidate clustering
4. **WL graph kernel** similarity → threshold **0.9** for final clone verdict

**How cyclo implements it:** Exact. All three thresholds match the paper:
- `charVecThreshold = 0.9` ("we set a threshold of 0.9 for numerical similarity filtering")
- `ccStage2NameThreshold = 0.5` ("we only filter those PDG pairs which Jaro-Winkler distance ratio less than 0.5")
- `ccMatchThreshold = 900` (0.9, "we verify the two PDGs are code clones when their similarity is greater than or equal to 0.9")

**What cyclo added beyond the paper:**
- Parallelized pairwise stages (Jaro-Winkler filter and WL kernel verification) across `runtime.NumCPU()` workers
- Slice-indexed inner loops (eliminated per-pair map lookups)
- Hardware `math.Sqrt` replacing a custom Newton-method sqrt in cosine similarity
- The paper's order is preserved exactly — parallelism doesn't change results

**In cyclo:** `ccgraph_clone` pattern kind. `domain/patterns/ccgraph.go`, `domain/patterns/wl.go`, `domain/patterns/lsh.go`.

**Note:** CCGraph replaced cyclo's earlier `semantic_clone` implementation (Komondoor & Horwitz style BFS subgraph enumeration), which caused 6-minute/4GB hangs on large codebases. The old code is deleted.

---

### LSH-Vectorized Clone Discovery — Gabel, Jiang & Su, ICSE 2008

**Paper:** "Scalable detection of semantic clones"
**DOI:** 10.1145/1368088.1368132

**The insight:** Subgraph isomorphism is too slow at repository scale. Map each PDG
subgraph to a characteristic vector (capturing node labels + structural context),
then cluster vectors with locality-sensitive hashing. Similar vectors land in the
same buckets without O(n²) pairwise comparison. Finds Type-4 (semantic) clones —
code that's semantically equivalent but syntactically different.

**How cyclo uses it:** The LSH layer inside CCGraph. After the two-stage filter,
WL histograms are vectorized to 512 dimensions and clustered with LSH (16 tables,
4 hashes, deterministic seed). Only bucket-mates get the expensive WL kernel
verification.

**Fidelity:** Adapted. Gabel's paper is about the vectorization approach generally;
cyclo uses it as CCGraph's scaling layer specifically.

**In cyclo:** `domain/patterns/lsh.go`. Referenced in `ccgraph.go` as "Scaling layer (Gabel et al.)".

---

### CP-Miner — Li, Lu, Myagmar & Zhou, OSDI 2004

**Paper:** "CP-Miner: Finding Copy-Paste and Related Bugs in Large-Scale Software Code"

**The insight:** Find copy-pasted code, then check whether the copies were
*consistently* modified. Inconsistent edits to clones are bugs — e.g., someone
fixed a nil check in one copy but forgot the pasted copies.

**How cyclo implements it:** Rebuilt on CCGraph groups (the original implementation
depended on the deleted BFS enumeration). Within each CCGraph clone group, compute
pairwise WL similarity. If a pair scores in [900, 950) — above the clone threshold
but below the "identical" range — it's flagged as suspiciously diverged.

**Fidelity:** Adapted. The core "inconsistent edits to clones = bugs" idea is
CP-Miner's; the divergence detection via WL similarity bands is cyclo's.

**In cyclo:** `inconsistent_clone` pattern kind (detection-only). `domain/patterns/inconsistent.go`.

---

### PR-Miner — Li & Zhou, FSE 2005

**Paper:** "PR-Miner: Automatically Extracting Implicit Programming Rules and Detecting Violations in Large Software Code"

**The insight:** Frequent itemset mining over function-call co-occurrences extracts
*implicit* programming rules with zero annotations. "In this repo, `sql.Query`
results are always closed." Found 16 confirmed bugs in Linux. The self-supervised
version of every hand-written linter rule.

**How cyclo implements it:** Mine rules of form `{A} → B` (if you call A, you
usually also call B). Support ≥10%, confidence ≥90%. Violations are flagged as
bugs.

**Fidelity:** Adapted. Core frequent-itemset mining approach; thresholds tuned for
cyclo's use case.

**In cyclo:** `mined_rule` pattern kind. `domain/patterns/pr_miner.go`.

---

### Bugs as Deviant Behavior — Engler, Chen, Hallem, Chou & Chelf, SOSP 2001

**Paper:** "Bugs as Deviant Behavior: A General Approach to Inferring Errors in Systems Code"

**The insight:** Extract programmer *beliefs* from code, then find contradictions.
MUST beliefs (always true — violations are always bugs) vs. MAY beliefs
(statistically true — violations ranked by deviation). You don't need to know the
truth; a contradiction IS the bug. Engler commercialized this as Coverity.

**How cyclo implements it:** Mine beliefs about error handling: if 95%+ of callers
check a function's error return, the sites that don't are flagged. Uses a
parent-tracking AST walker to detect `_` assignments and ignored results.

**Fidelity:** Inspired by. The MUST/MAY belief distinction is Engler's; cyclo's
implementation focuses specifically on error-handling beliefs.

**In cyclo:** `deviant_behavior` pattern kind. `domain/patterns/deviant.go`, `adapters/gopatterns/errorcheck.go`.

---

### Alattin — Thummalapenta & Xie, ASE 2009

**Paper:** "Alattin: Mining Alternative Patterns for Detecting Neglected Conditions"

**The insight:** Classic belief miners (PR-Miner, Engler) find X→Y and flag ¬Y as
bugs. But API preconditions are usually disjunctive: "check arg for null OR check
return for error" — not "check both." Alattin mines *alternative* (OR) patterns
and only flags code that violates *all* alternatives. Cut false positives ~28%.

**How cyclo implements it:** Disjunctive rules over call sets. Where PR-Miner mines
"if you call A, you also call B", Alattin mines "if you call A, you call B OR C".
The OR rule only fires when neither single rule meets the confidence threshold.

**Fidelity:** Adapted. Core disjunctive mining idea; implemented on cyclo's PDG
call facts.

**In cyclo:** `alattin_rule` pattern kind (detection-only). `domain/patterns/alattin.go`.

---

### Barrier Slicing — Krinke, SQJ 2004

**Paper:** "Barrier Slicing and Chopping"

**The insight:** Declare parts of the dependence graph off-limits, and the slice
can't transitively pass through them. Unifies two ad-hoc practices: removing
dependences from the graph, and dicing (removing known-good code from a slice).

**How cyclo implements it:** `BarrierSlice(pdg, seed, barriers)` computes the
backward slice from a seed without traversing through barrier nodes. Use case:
the agent marks reviewed code as a barrier, and the fault area shrinks each
iteration.

**Fidelity:** Adapted. Core barrier concept; implemented on cyclo's PDG.

**In cyclo:** Library only (`domain/patterns/barrier.go`). Not yet wired as a CLI pattern kind.

---

### Error Handling Obligations — Weimer & Necula, OOPSLA 2004

**Paper:** "Finding and Preventing Run-Time Error Handling Mistakes"

**The insight:** Flow-sensitive dataflow tracks outstanding *obligations* —
resources that must be released on *every* path, including error paths. Error
paths are where resource discipline breaks. The paper found 800+ error-handling
mistakes in ~4M LOC of Java from pure static analysis.

**How cyclo implements it:** Tracks resource acquisition (open, lock, allocate)
and verifies release (close, unlock, free) on all paths including error returns.
Go's `defer` makes the common case easy; the analysis catches the cases where
defer is missing or conditional.

**Fidelity:** Adapted. Core obligation-tracking idea; adapted for Go's error-handling
idioms.

**In cyclo:** `obligation` pattern kind (detection-only). `domain/patterns/obligation.go`.

---

### Chopping — Jackson & Rollins, 1994

**Paper:** "Chopping: A Generalization of Slicing" (CMU-CS-94-169)

**The insight:** A *chop* between a source and a sink is the intersection of the
forward slice of the source with the backward slice of the sink. Answers "how does
THIS input reach THAT output?" rather than "what affects X?"

**How cyclo implements it:** `Chop(pdg, source, sink)`. For `check --changed`:
given changed lines (sources) and sinks (DB writes, network calls), show exactly
the statements on the path between them.

**Fidelity:** Adapted. Core chop definition; implemented on cyclo's PDG.

**In cyclo:** Library only (`domain/patterns/chop.go`).

---

### Thin Slicing — Sridharan, Fink & Bodik, PLDI 2007

**Paper:** "Thin Slicing"

**The insight:** Traditional slices are too big because they include everything
that MIGHT affect a value — control dependences, heap plumbing. A *thin slice*
includes only PRODUCER statements: the chain of assignments that compute and copy
the value. 3-9x fewer statements; the buggy statement is still included.

**How cyclo implements it:** `ThinSlice(pdg, seed)` follows only Data edges
backward. When explaining a candidate, show the thin slice (5 lines) instead of
the full dependence cone (50).

**Fidelity:** Adapted. The paper has a more precise definition; cyclo uses a
heuristic (Data edges only).

**In cyclo:** Library only (`domain/patterns/thin_slice.go`).

---

### Dependence Clusters — Binkley & Harman, ICSME 2015

**Papers:** "Coherent Dependence Clusters" (JSS); "Dependence Cluster Visualization" (ICSME 2015)

**The insight:** A *dependence cluster* is a set of statements that are mutually
dependent (strongly connected in the dependence graph). Large programs contain
clusters spanning 10%+ of the codebase. Big clusters = "everything affects
everything" spaghetti. *Linchpin functions* are the ones whose removal collapses
a cluster — highest-leverage refactoring targets.

**How cyclo implements it:** Tarjan's SCC algorithm finds mutually-dependent
regions. Flags functions where 10+ statements are tangled, or 25%+ of the
function is one cluster.

**Fidelity:** Adapted. Core SCC-based cluster detection; thresholds chosen for
cyclo.

**In cyclo:** `dependence_cluster` pattern kind (detection-only). `domain/patterns/dep_cluster.go`.

---

### Semantic Diff — Horwitz, PLDI 1990

**Paper:** "Identifying the Semantic and Textual Differences Between Two Versions of a Program"

**The insight:** Compare versions by their *dependence graphs*, not their text.
A refactoring that moves code produces zero semantic diff; a one-line change
that alters dependences is flagged as significant.

**How cyclo implements it:** `SemanticDiff(old, new)` compares PDG structure
hashes ignoring line numbers. For `check --changed`: pure refactors get a free
pass; behavior changes get scrutiny.

**Fidelity:** Inspired by. The "compare PDGs not text" idea; cyclo's implementation
is simpler than Horwitz's full partitioning.

**In cyclo:** Library only (`domain/patterns/semantic_diff.go`).

---

### Anti-Unification — Bulychev & Minea, 2008

**Paper:** "Duplicate Code Detection Using Anti-Unification"

**The insight:** Anti-unification is the mathematical dual of unification. Given
two similar fragments, formally derive the template + minimal parameter list.
The *provably correct* way to extract a helper — not heuristics, but the
most-specific generalization.

**How cyclo implements it:** Drives the `parameterize` fixer. Instead of
heuristic diffing, anti-unification derives the exact template and holes, which
become the extracted function's parameters. Respects variable renames.

**Fidelity:** Exact. Standard anti-unification algorithm.

**In cyclo:** `domain/patterns/antiunify.go`. Wired into the `parameterize` fixer.

---

## Taint Analysis (SAST literature)

**Pattern kind:** `taint_flow` (detection-only)

Taint analysis doesn't come from a single paper — it's the standard
source→propagator→sink framework from security-focused static analysis (SAST)
engineering. Cyclo's implementation:

- **Sources:** `r.URL.Query().Get()`, `r.FormValue()`, `os.Getenv()`, `io.ReadAll()`, etc.
- **Propagators:** String concatenation, assignment, function returns (conservative)
- **Sanitizers:** `template.HTMLEscapeString()`, `url.QueryEscape()`, `strconv` numeric parsing
- **Sinks:** `db.Exec()`/`Query()` (arg 1 only — parameterized queries are safe), `exec.Command()`, `template.Execute()`, `os.Open()` (path arg)

**Key design decision:** Only flags tainted data in the *query string* argument
(arg 1) of SQL calls. `db.Exec("... WHERE id = ?", userID)` is the safe pattern
and is NOT flagged. `db.Exec("... WHERE id = '" + userID + "'")` IS flagged.

**Algorithm:** Intraprocedural forward dataflow along PDG data edges. Seed sources
as tainted, propagate to fixpoint, flag sinks with tainted dangerous args.

**In cyclo:** `domain/patterns/taint.go`. Wired into `cyclo patterns` CLI.

---

## Removed

### Semantic Clones via BFS Enumeration — Komondoor & Horwitz, SAS 2001

**Paper:** "Using Slicing to Identify Duplication in Source Code"

**What it was:** Found isomorphic PDG subgraphs via breadth-first search
enumeration of 10-12 node subgraphs, with WL hashing + isomorphism verification.

**Why it was removed:** The BFS enumeration caused 6-minute/4GB hangs on
codebases with 2,500+ functions. CCGraph (above) replaces it with better
recall, better precision, and orders of magnitude better performance.

**Status:** Deleted. `ccgraph_clone` is the only clone detector.

---

## Not Yet Implemented

### The PDG Foundation — Ferrante, Ottenstein & Warren, TOPLAS 1987

"The Program Dependence Graph and Its Use in Optimization." The paper that
invented the PDG. Everything above builds on this.

### Interprocedural Slicing — Horwitz, Reps & Binkley, 1988-1990

System dependence graphs, precise interprocedural slicing. Cyclo's PDG is
currently intraprocedural; this is the path to cross-function analysis.

---

## The Thesis

Cyclo is the AI's guide for attention and static analysis. These papers are the
buried gold — old ideas that were right but too slow, too manual, or too
academic to productize. Modern compute + cyclo's PDG extractor unlocks them.

The pattern: **static analysis tells the agent where to look; the agent decides
what to do.** Detection is deterministic. Fixes are conservative. Attention is
the scarce resource.

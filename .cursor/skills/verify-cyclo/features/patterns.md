# Pattern mining

`cyclo patterns` mines Go code for 18 kinds of mechanical patterns — structural
issues with deterministic AST fixes, from guard clauses and value objects to
DDD patterns like specifications and domain services. Text output lists one
candidate per block with its observation, inference, and suggested refactor;
exit code is always 0 (informational, never a gate).

## Sub-features

- `patterns-text` reports candidates as `#N kind score [support, lift, holes, coverage]` with observation, inference, possible refactor, sites, and counter evidence.
- `patterns-kinds` covers all 18 kinds: guard_clause, value_object, parameterize, anemic_model, primitive_obsession, type_switch, enum_dispatch, trait_method, capability_set, generic_fn, entity_identity, missing_identity, mutable_identity, aggregate, repository, factory, specification, domain_service.
- `patterns-detection-only` — aggregate, repository, mutable_identity, and domain_service are detection-only (nil FixSpec); they inform but never auto-fix.
- `patterns-suppression` — domain_service suppresses the corresponding anemic_model suggestions for stateless multi-type functions.

## How to get to it (user POV)

- Run `cyclo patterns .` for the current directory.
- Run `cyclo patterns ./domain` for one package.
- Pipe to `grep` for one kind: `cyclo patterns . | grep -A5 trait_method`.

## Driving it with verify-cyclo

Preconditions: `verify-cyclo build "$RUN_DIR"` and `verify-cyclo doctor "$RUN_DIR/cyclo"` report ok; `go` on PATH.

- Craft a fixture with a known pattern: two types with parallel methods for `trait_method`. Use this exact shape (blank lines and method order matter to the miner):
  ```go
  package main

  import "fmt"

  // Parallel types whose Speak methods differ only in the helper they call.
  type Dog struct{ name string }

  func (d Dog) Speak() string {
      s := d.sound()
      return fmt.Sprintf("%s says %s", d.name, s)
  }

  func (d Dog) sound() string { return "woof" }

  type Cat struct{ name string }

  func (c Cat) Speak() string {
      s := c.sound()
      return fmt.Sprintf("%s says %s", c.name, s)
  }

  func (c Cat) sound() string { return "meow" }

  func main() {
      fmt.Println(Dog{name: "rex"}.Speak())
      fmt.Println(Cat{name: "tom"}.Speak())
  }
  ```
  Save under `$RUN_DIR/scratch/patterns/` with a `go.mod` (`module patterntest`, `go 1.21`).
- Run `$RUN_DIR/cyclo patterns $RUN_DIR/scratch/patterns` and assert the output contains `trait_method` with both sites listed. Exit code is 0.
- For a negative case, put the same-named methods on types in *different* packages; assert no `trait_method` candidate (cross-package collisions are skipped).
- For `specification`, craft two functions with the same boolean rule (`u.Age > 18 && u.Active`); assert a `specification` candidate naming the type.
- For a two-variable rule (`c.x != first.x`), assert no `specification` candidate (would extract with a dangling reference).
- Save stdout to `$RUN_DIR/evidence/patterns-<id>.txt` with the command and exit code.

## Gotchas

- The miner needs real type information: fixtures must be valid Go packages with a `go.mod`, or `patterns` reports load errors instead of candidates.
- `trait_method` requires 2+ distinct self types *in the same package*; the SelfTy field must be populated by the extractor (a past bug left it empty and the miner silently found nothing — if `patterns` reports 0 candidates on a fixture that should match, check SelfTy first).
- `specification` only fires on single-subject rules (one variable); multi-variable conditions are skipped by design.
- Detection-only kinds appear in `patterns` output but never in `fix` dry-runs.

.PHONY: check complexity cognitive cyclo-check fmt lint quality-baseline quality-gate test test-race

check:
	go test -buildvcs=false ./...
	$(MAKE) complexity

complexity:
	go run -buildvcs=false github.com/fzipp/gocyclo/cmd/gocyclo -over 6 -ignore '_test.go' .

cognitive:
	go run -buildvcs=false github.com/uudashr/gocognit/cmd/gocognit -over 15 -ignore '_test.go' .

# Dogfood gate: cyclo's two dimensions (tile area = cyclomatic, color =
# cognitive) must each stay under 15 on non-test code. Kept separate from
# lint so the TUI's own health is its own CI signal.
cyclo-check:
	go run -buildvcs=false github.com/fzipp/gocyclo/cmd/gocyclo -over 15 -ignore '_test.go' .
	go run -buildvcs=false github.com/uudashr/gocognit/cmd/gocognit -over 15 -ignore '_test.go' .

# Quality gate: cyclo's guardrails (fn_params, mutated targets, side-effect
# density, ...) as excess rates per 1,000 functions, so the gate scales with
# the codebase instead of pinning absolute finding counts. `check` exits 1
# when it reports findings, which is expected; only exit 2 (analysis
# failure) fails the run.
quality-gate:
	go build -buildvcs=false -o /tmp/cyclo-check-bin .
	/tmp/cyclo-check-bin check --format json . > /tmp/cyclo-facts.json || test $$? -eq 1
	go run ./application/qualitygate --baseline quality-baseline.json --facts /tmp/cyclo-facts.json

# Regenerate the checked-in baseline after a change legitimately moves the
# numbers (usually down).
quality-baseline:
	go build -buildvcs=false -o /tmp/cyclo-check-bin .
	/tmp/cyclo-check-bin check --format json . > /tmp/cyclo-facts.json || test $$? -eq 1
	go run ./application/qualitygate --write quality-baseline.json --facts /tmp/cyclo-facts.json

fmt:
	go fmt ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet -buildvcs=false ./...
	$(MAKE) complexity

test:
	go test -buildvcs=false -count=1 ./...

test-race:
	go test -buildvcs=false -race -count=1 ./...

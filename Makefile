.PHONY: check complexity cognitive cyclo-check fmt lint test test-race

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

package domainservice

// Account is a domain entity.
type Account struct {
	ID      string
	Balance int
}

// Money is a value object.
type Money struct {
	Amount   int
	Currency string
}

// Transfer operates on two domain types without belonging to either:
// a textbook domain service (Evans).
func Transfer(from *Account, to *Account, m Money) {
	from.Balance -= m.Amount
	to.Balance += m.Amount
}

// SingleType operates on only one struct: not a service.
func SingleType(a *Account) int {
	return a.Balance
}

// GlobalSink assigns a package-level var: not stateless, not a service.
var sink int

func GlobalSink(a *Account, m Money) {
	sink = a.Balance + m.Amount
}

// NoFieldAccess takes two structs but touches no fields: not a service.
func NoFieldAccess(a *Account, m Money) int {
	return 42
}

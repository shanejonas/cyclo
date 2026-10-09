package args

const DefaultLimit = 10

type Config struct {
	Verbose bool
	Name    string
}

func target(a int, b string, c bool) int { return a }

func targetPtr(a *int, b string) int { return *a }

func caller(p1 int, p2 string, cfg Config) int {
	n := target(p1, "lit", true)        // param(0), const, const
	m := target(p1, p2, cfg.Verbose)    // param(0), param(1), field
	k := target(DefaultLimit, p2, true) // const, param(1), const
	j := targetPtr(&p1, p2)             // param(0) peeled &, param(1)
	h := target(p1+1, p2, true)         // other, param(1), const
	e := target(len(p2), p2, true)      // other, param(1), const
	_ = n + m + k + j + h + e
	return n
}

func (c Config) method(p string) int {
	return target(1, p, c.Verbose) // const, param(0), field via receiver
}

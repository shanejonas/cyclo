package shapes

// SumFor uses a C-style for loop with indexing.
func SumFor(names []string) int {
	total := 0
	for i := 0; i < len(names); i++ {
		total += len(names[i])
	}
	return total
}

// SumRange uses range over the same collection and body shape. The loop-element
// canonicalization should make this match SumFor.
func SumRange(names []string) int {
	total := 0
	for _, n := range names {
		total += len(n)
	}
	return total
}

// SumWhile uses a condition loop: a Loop, not an Iterate.
func SumWhile(names []string) int {
	total := 0
	i := 0
	for i < len(names) {
		total += len(names[i])
		i++
	}
	return total
}

// ClosureDemo has an opaque func literal.
func ClosureDemo(items []string) int {
	count := 0
	each := func(s string) {
		count++
	}
	for _, item := range items {
		each(item)
	}
	return count
}

// DeferDemo defers cleanup: the deferred call must be a Defer node.
func DeferDemo(db *DB) error {
	db.Ping()
	defer db.Ping()
	return nil
}

// GoDemo launches a goroutine: the spawned call must be a Go node.
func GoDemo(db *DB) {
	go db.Ping()
	db.Ping()
}

// MultiReturn exercises multi-value returns and destructuring.
func MultiReturn(a, b int) (int, int, error) {
	sum := a + b
	diff := a - b
	if sum == 0 {
		return 0, 0, nil
	}
	return sum, diff, nil
}

// MethodDemo exercises method calls with receivers at arg 0.
func (db *DB) MethodDemo(query string) error {
	_, err := db.Exec(query)
	if err != nil {
		return err
	}
	return db.Ping()
}

// SwitchDemo exercises a value switch (Match node).
func SwitchDemo(kind string) string {
	switch kind {
	case "a":
		return "alpha"
	case "b":
		return "beta"
	default:
		return "other"
	}
}

// TypeSwitchDemo exercises a type switch (Match node with binding).
func TypeSwitchDemo(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return "int"
	default:
		return "other"
	}
}

// TryVariants covers the Try idiom in its canonical form.
func TryVariants(db *DB) error {
	id, err := insertRow(db)
	if err != nil {
		return err
	}
	return db.PingRow(id)
}

func insertRow(db *DB) (int64, error) { return 0, nil }
func (db *DB) PingRow(id int64) error { return nil }

package shapes

import (
	"errors"
	"time"
)

type DB struct{}

func (db *DB) Exec(query string, args ...string) (int64, error) { return 0, nil }
func (db *DB) Ping() error                                      { return nil }

// CreateUser validates then inserts: the validate-then-act shape.
func CreateUser(db *DB, name string, email string) error {
	if name == "" {
		return errors.New("name required")
	}
	if email == "" {
		return errors.New("email required")
	}
	_, err := db.Exec("INSERT INTO users", name, email)
	if err != nil {
		return err
	}
	return nil
}

// CreateOrder is structurally parallel to CreateUser: different names and
// types, same validate-then-act shape. The miner should pair them.
func CreateOrder(db *DB, item string, qty int) error {
	if item == "" {
		return errors.New("item required")
	}
	if qty <= 0 {
		return errors.New("qty must be positive")
	}
	_, err := db.Exec("INSERT INTO orders", item)
	if err != nil {
		return err
	}
	return nil
}

// FetchWithRetry retries an operation with backoff.
func FetchWithRetry(url string) (string, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		body, err := fetchURL(url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return "", lastErr
}

// LoadWithRetry is structurally parallel to FetchWithRetry: same retry shape,
// same signature classes, different names.
func LoadWithRetry(key string) (string, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		value, err := loadKey(key)
		if err == nil {
			return value, nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return "", lastErr
}

func fetchURL(url string) (string, error) { return "", nil }
func loadKey(key string) (string, error)  { return "", nil }

// NormalizeScores is unrelated to the validate-then-act pair: a negative
// control for similarity.
func NormalizeScores(scores []float64) []float64 {
	total := 0.0
	for _, s := range scores {
		total += s
	}
	out := make([]float64, len(scores))
	for i, s := range scores {
		out[i] = s / total
	}
	return out
}

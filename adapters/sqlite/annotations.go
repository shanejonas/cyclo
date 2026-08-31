package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shanejonas/cyclo/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	database *sql.DB
}

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is required")
	}
	err := os.MkdirAll(filepath.Dir(path), 0o700)
	if err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	database.SetMaxOpenConns(1)
	store := &Store{database: database}
	err = store.migrate()
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

func StatePath() (string, error) {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome != "" {
		return filepath.Join(stateHome, "cyclo", "annotations.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "cyclo", "annotations.db"), nil
}

func (s *Store) Close() error {
	return s.database.Close()
}

func (s *Store) ListAnnotations(repository string) ([]domain.Annotation, error) {
	rows, err := s.database.Query(`
		SELECT id, path, function_name, function_line, start_line, end_line, message, text
		FROM annotations WHERE repository = ? ORDER BY rowid
	`, repository)
	if err != nil {
		return nil, fmt.Errorf("query annotations: %w", err)
	}
	defer rows.Close()

	annotations := []domain.Annotation{}
	for rows.Next() {
		var annotation domain.Annotation
		err = rows.Scan(
			&annotation.ID, &annotation.Path, &annotation.Function, &annotation.FunctionLine,
			&annotation.StartLine, &annotation.EndLine, &annotation.Message, &annotation.Text,
		)
		if err != nil {
			return nil, fmt.Errorf("scan annotation: %w", err)
		}
		annotations = append(annotations, annotation)
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("read annotations: %w", err)
	}
	return annotations, nil
}

func (s *Store) SaveAnnotation(repository string, annotation domain.Annotation) error {
	_, err := s.database.Exec(`
		INSERT INTO annotations (
			repository, id, path, function_name, function_line, start_line, end_line, message, text
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, repository, annotation.ID, annotation.Path, annotation.Function, annotation.FunctionLine,
		annotation.StartLine, annotation.EndLine, annotation.Message, annotation.Text)
	if err != nil {
		return fmt.Errorf("insert annotation: %w", err)
	}
	return nil
}

func (s *Store) DeleteAnnotation(repository string, id string) error {
	result, err := s.database.Exec("DELETE FROM annotations WHERE repository = ? AND id = ?", repository, id)
	if err != nil {
		return fmt.Errorf("delete annotation: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted annotations: %w", err)
	}
	if deleted == 0 {
		return errors.New("annotation not found")
	}
	return nil
}

func (s *Store) migrate() error {
	_, err := s.database.Exec(`
		PRAGMA journal_mode = WAL;
		CREATE TABLE IF NOT EXISTS annotations (
			repository TEXT NOT NULL,
			id TEXT NOT NULL,
			path TEXT NOT NULL,
			function_name TEXT NOT NULL,
			function_line INTEGER NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			message TEXT NOT NULL,
			text TEXT NOT NULL,
			PRIMARY KEY (repository, id)
		);
	`)
	if err != nil {
		return fmt.Errorf("migrate SQLite: %w", err)
	}
	return nil
}

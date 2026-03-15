package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/mattn/go-sqlite3" // CGO SQLite driver
)

// Status values for the jobs table.
const (
	StatusPending = "pending"
	StatusApplied = "applied"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
)

// Job represents a single job listing record in the database.
type Job struct {
	ID          int64
	Title       string
	Company     string
	URL         string
	Description string
	MatchScore  float64
	Status      string
	AppliedAt   *time.Time
}

// DB wraps a sql.DB connection and provides domain-level operations.
type DB struct {
	conn   *sql.DB
	logger *slog.Logger
}

// New opens (or creates) the SQLite database at the given path,
// runs schema migrations, and returns a ready DB handle.
func New(path string, logger *slog.Logger) (*DB, error) {
	conn, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_journal_mode=WAL&_foreign_keys=on", path))
	if err != nil {
		return nil, fmt.Errorf("db: open %q: %w", path, err)
	}

	// Limit to one writer to avoid "database is locked" under WAL
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn, logger: logger}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("db: migration failed: %w", err)
	}

	logger.Info("database ready", "path", path)
	return db, nil
}

// Close releases the underlying database connection.
func (d *DB) Close() error {
	return d.conn.Close()
}

// migrate runs idempotent schema creation statements.
func (d *DB) migrate() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS jobs (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		title       TEXT    NOT NULL,
		company     TEXT    NOT NULL,
		url         TEXT    NOT NULL UNIQUE,
		description TEXT,
		match_score REAL    NOT NULL DEFAULT 0,
		status      TEXT    NOT NULL DEFAULT 'pending',
		applied_at  DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs (status);
	CREATE INDEX IF NOT EXISTS idx_jobs_url    ON jobs (url);
	`

	_, err := d.conn.Exec(schema)
	return err
}

// SaveJob inserts a new job record. If a record with the same URL already
// exists (UNIQUE constraint), the insert is silently ignored so the caller
// can treat deduplication as a simple boolean check via ExistsURL.
func (d *DB) SaveJob(job *Job) error {
	const q = `
	INSERT OR IGNORE INTO jobs (title, company, url, description, match_score, status)
	VALUES (?, ?, ?, ?, ?, ?)
	`
	res, err := d.conn.Exec(q, job.Title, job.Company, job.URL, job.Description, job.MatchScore, job.Status)
	if err != nil {
		return fmt.Errorf("db: SaveJob %q: %w", job.URL, err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		d.logger.Debug("job already exists, skipping insert", "url", job.URL)
	}
	return nil
}

// ExistsURL returns true if a job with the given URL is already recorded.
// Used for fast deduplication before running the expensive AI evaluation.
func (d *DB) ExistsURL(url string) (bool, error) {
	var count int
	err := d.conn.QueryRow(`SELECT COUNT(1) FROM jobs WHERE url = ?`, url).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("db: ExistsURL %q: %w", url, err)
	}
	return count > 0, nil
}

// UpdateStatus sets the status (and optionally applied_at) for a given job URL.
func (d *DB) UpdateStatus(url, status string) error {
	var err error
	if status == StatusApplied {
		_, err = d.conn.Exec(
			`UPDATE jobs SET status = ?, applied_at = ? WHERE url = ?`,
			status, time.Now().UTC(), url,
		)
	} else {
		_, err = d.conn.Exec(
			`UPDATE jobs SET status = ? WHERE url = ?`,
			status, url,
		)
	}
	if err != nil {
		return fmt.Errorf("db: UpdateStatus %q → %q: %w", url, status, err)
	}
	d.logger.Info("job status updated", "url", url, "status", status)
	return nil
}

// UpdateMatchScore stores the AI-evaluated score for a job URL.
func (d *DB) UpdateMatchScore(url string, score float64) error {
	_, err := d.conn.Exec(`UPDATE jobs SET match_score = ? WHERE url = ?`, score, url)
	if err != nil {
		return fmt.Errorf("db: UpdateMatchScore %q: %w", url, err)
	}
	return nil
}

// ListByStatus returns all job records with the given status.
func (d *DB) ListByStatus(status string) ([]Job, error) {
	rows, err := d.conn.Query(
		`SELECT id, title, company, url, description, match_score, status, applied_at FROM jobs WHERE status = ?`,
		status,
	)
	if err != nil {
		return nil, fmt.Errorf("db: ListByStatus %q: %w", status, err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.Title, &j.Company, &j.URL, &j.Description,
			&j.MatchScore, &j.Status, &j.AppliedAt,
		); err != nil {
			return nil, fmt.Errorf("db: scan row: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// Stats returns a snapshot of job counts by status for the run summary.
func (d *DB) Stats() (map[string]int, error) {
	rows, err := d.conn.Query(`SELECT status, COUNT(1) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("db: Stats: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
	}
	return result, rows.Err()
}

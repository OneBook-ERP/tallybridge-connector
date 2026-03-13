package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// LocalJob represents a pending write operation destined for Tally.
// Jobs are buffered here when the cloud cannot be reached.
type LocalJob struct {
	ID         string    // UUID
	JobType    string    // CREATE | ALTER | CANCEL
	Payload    string    // JSON-encoded job payload
	CreatedAt  time.Time
	RetryCount int
	Status     string // PENDING | SENT | FAILED
}

// SaveJobToQueue inserts a new job with status PENDING.
// The job.ID must be set by the caller (UUID).
func (s *Store) SaveJobToQueue(job LocalJob) error {
	_, err := s.db.Exec(
		`INSERT INTO job_queue (id, job_type, payload, created_at, retry_count, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		job.ID, job.JobType, job.Payload, job.CreatedAt.UTC(), job.RetryCount, "PENDING",
	)
	if err != nil {
		return fmt.Errorf("store: SaveJobToQueue: %w", err)
	}
	return nil
}

// GetPendingJobs returns all jobs with status PENDING, oldest first.
func (s *Store) GetPendingJobs() ([]LocalJob, error) {
	rows, err := s.db.Query(
		`SELECT id, job_type, payload, created_at, retry_count, status
		 FROM job_queue
		 WHERE status = 'PENDING'
		 ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: GetPendingJobs: %w", err)
	}
	defer rows.Close()

	var jobs []LocalJob
	for rows.Next() {
		var j LocalJob
		var createdAtStr string
		if err := rows.Scan(
			&j.ID, &j.JobType, &j.Payload, &createdAtStr, &j.RetryCount, &j.Status,
		); err != nil {
			return nil, fmt.Errorf("store: GetPendingJobs scan: %w", err)
		}
		// SQLite stores datetime as text; parse it back.
		j.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAtStr)
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// MarkJobSent transitions a job's status from PENDING to SENT.
func (s *Store) MarkJobSent(jobId string) error {
	res, err := s.db.Exec(
		`UPDATE job_queue SET status = 'SENT' WHERE id = ? AND status = 'PENDING'`,
		jobId,
	)
	if err != nil {
		return fmt.Errorf("store: MarkJobSent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: MarkJobSent: job %q not found or not PENDING", jobId)
	}
	return nil
}

// IncrementRetry bumps the retry counter and marks a job FAILED if it
// exceeds maxRetries. Used by the executor on Tally write failure.
func (s *Store) IncrementRetry(jobId string, maxRetries int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: IncrementRetry: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var current int
	if err := tx.QueryRow(
		`SELECT retry_count FROM job_queue WHERE id = ?`, jobId,
	).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: IncrementRetry: job %q not found", jobId)
		}
		return fmt.Errorf("store: IncrementRetry: %w", err)
	}

	newCount := current + 1
	newStatus := "PENDING"
	if newCount >= maxRetries {
		newStatus = "FAILED"
	}

	if _, err := tx.Exec(
		`UPDATE job_queue SET retry_count = ?, status = ? WHERE id = ?`,
		newCount, newStatus, jobId,
	); err != nil {
		return fmt.Errorf("store: IncrementRetry: %w", err)
	}

	return tx.Commit()
}

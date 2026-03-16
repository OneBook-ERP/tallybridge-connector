package store

import (
	"testing"
	"time"
)

// ── Group A: Job Queue CRUD ──────────────────────────────────────────────────

func TestJobQueue_SaveAndGetPending(t *testing.T) {
	s := testStore(t)

	job := LocalJob{
		ID:        "uuid-001",
		JobType:   "CREATE",
		Payload:   `{"voucherType":"Sales Invoice"}`,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.SaveJobToQueue(job); err != nil {
		t.Fatalf("SaveJobToQueue: %v", err)
	}

	jobs, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("expected 1 pending job, got %d", len(jobs))
	}
	j := jobs[0]
	if j.ID != "uuid-001" {
		t.Errorf("ID: got %q, want %q", j.ID, "uuid-001")
	}
	if j.JobType != "CREATE" {
		t.Errorf("JobType: got %q, want %q", j.JobType, "CREATE")
	}
	if j.Payload != `{"voucherType":"Sales Invoice"}` {
		t.Errorf("Payload mismatch: got %q", j.Payload)
	}
	if j.Status != "PENDING" {
		t.Errorf("Status: got %q, want PENDING", j.Status)
	}
	if j.RetryCount != 0 {
		t.Errorf("RetryCount: got %d, want 0", j.RetryCount)
	}
}

func TestJobQueue_MarkJobSent(t *testing.T) {
	s := testStore(t)

	job := LocalJob{
		ID:        "uuid-sent",
		JobType:   "ALTER",
		Payload:   `{}`,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.SaveJobToQueue(job); err != nil {
		t.Fatalf("SaveJobToQueue: %v", err)
	}

	if err := s.MarkJobSent("uuid-sent"); err != nil {
		t.Fatalf("MarkJobSent: %v", err)
	}

	// Pending list should now be empty
	jobs, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 pending jobs after MarkJobSent, got %d", len(jobs))
	}
}

func TestJobQueue_MarkJobSent_NotFound(t *testing.T) {
	s := testStore(t)

	// Marking a non-existent job should return an error
	err := s.MarkJobSent("no-such-id")
	if err == nil {
		t.Error("expected error for non-existent job ID, got nil")
	}
}

func TestJobQueue_MarkJobSent_AlreadySent(t *testing.T) {
	s := testStore(t)

	job := LocalJob{ID: "uuid-double", JobType: "CREATE", Payload: `{}`, CreatedAt: time.Now().UTC()}
	_ = s.SaveJobToQueue(job)
	_ = s.MarkJobSent("uuid-double")

	// Second MarkJobSent on a SENT job should error (no rows affected)
	err := s.MarkJobSent("uuid-double")
	if err == nil {
		t.Error("expected error marking already-SENT job, got nil")
	}
}

func TestJobQueue_IncrementRetry_BelowMax(t *testing.T) {
	s := testStore(t)

	job := LocalJob{ID: "uuid-retry", JobType: "CREATE", Payload: `{}`, CreatedAt: time.Now().UTC()}
	if err := s.SaveJobToQueue(job); err != nil {
		t.Fatalf("SaveJobToQueue: %v", err)
	}

	// Increment once with maxRetries=3 — should stay PENDING
	if err := s.IncrementRetry("uuid-retry", 3); err != nil {
		t.Fatalf("IncrementRetry: %v", err)
	}

	jobs, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected job still PENDING, got %d", len(jobs))
	}
	if jobs[0].RetryCount != 1 {
		t.Errorf("RetryCount: got %d, want 1", jobs[0].RetryCount)
	}
	if jobs[0].Status != "PENDING" {
		t.Errorf("Status: got %q, want PENDING", jobs[0].Status)
	}
}

func TestJobQueue_IncrementRetry_AtMax_TransitionsToFailed(t *testing.T) {
	s := testStore(t)

	job := LocalJob{ID: "uuid-fail", JobType: "CREATE", Payload: `{}`, CreatedAt: time.Now().UTC()}
	if err := s.SaveJobToQueue(job); err != nil {
		t.Fatalf("SaveJobToQueue: %v", err)
	}

	// With maxRetries=2, incrementing once leaves count=1 (PENDING), twice → FAILED
	_ = s.IncrementRetry("uuid-fail", 2)
	if err := s.IncrementRetry("uuid-fail", 2); err != nil {
		t.Fatalf("second IncrementRetry: %v", err)
	}

	// Should no longer appear in PENDING
	jobs, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 pending jobs after maxRetries reached, got %d", len(jobs))
	}
}

func TestJobQueue_IncrementRetry_NotFound(t *testing.T) {
	s := testStore(t)

	err := s.IncrementRetry("no-such-id", 3)
	if err == nil {
		t.Error("expected error for non-existent job ID, got nil")
	}
}

func TestJobQueue_GetPendingJobs_OrderedOldestFirst(t *testing.T) {
	s := testStore(t)

	now := time.Now().UTC()
	old := LocalJob{ID: "uuid-old", JobType: "CREATE", Payload: `{}`, CreatedAt: now.Add(-10 * time.Minute)}
	mid := LocalJob{ID: "uuid-mid", JobType: "ALTER", Payload: `{}`, CreatedAt: now.Add(-5 * time.Minute)}
	new_ := LocalJob{ID: "uuid-new", JobType: "CANCEL", Payload: `{}`, CreatedAt: now}

	// Insert in reverse order to confirm ORDER BY works
	_ = s.SaveJobToQueue(new_)
	_ = s.SaveJobToQueue(old)
	_ = s.SaveJobToQueue(mid)

	jobs, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	want := []string{"uuid-old", "uuid-mid", "uuid-new"}
	for i, j := range jobs {
		if j.ID != want[i] {
			t.Errorf("position %d: got ID %q, want %q", i, j.ID, want[i])
		}
	}
}

func TestJobQueue_OnlyPendingJobsReturned(t *testing.T) {
	s := testStore(t)

	now := time.Now().UTC()
	_ = s.SaveJobToQueue(LocalJob{ID: "p1", JobType: "CREATE", Payload: `{}`, CreatedAt: now})
	_ = s.SaveJobToQueue(LocalJob{ID: "p2", JobType: "CREATE", Payload: `{}`, CreatedAt: now.Add(time.Second)})
	_ = s.MarkJobSent("p1") // p1 is now SENT

	jobs, err := s.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 pending (p2), got %d", len(jobs))
	}
	if jobs[0].ID != "p2" {
		t.Errorf("got %q, want p2", jobs[0].ID)
	}
}

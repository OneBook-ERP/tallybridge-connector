package store

import (
	"os"
	"path/filepath"
	"testing"
)

// testStore creates a temporary SQLite store for testing. Caller should defer cleanup.
func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInitDB_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.db")
	s, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected database file to be created")
	}
}

func TestInitDB_IdempotentMigrations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "idempotent.db")

	// First init
	s1, err := InitDB(path)
	if err != nil {
		t.Fatalf("first InitDB: %v", err)
	}
	s1.Close()

	// Second init should succeed (IF NOT EXISTS + INSERT OR IGNORE)
	s2, err := InitDB(path)
	if err != nil {
		t.Fatalf("second InitDB: %v", err)
	}
	s2.Close()
}

func TestConfig_SetAndGet(t *testing.T) {
	s := testStore(t)

	// Get before set returns empty (seeded keys have empty default)
	v := s.GetConfig("machine_token")
	if v != "" {
		t.Errorf("expected empty default, got %q", v)
	}

	// Set and get back
	if err := s.SetConfig("machine_token", "my-secret-token"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	v = s.GetConfig("machine_token")
	if v != "my-secret-token" {
		t.Errorf("got %q, want %q", v, "my-secret-token")
	}
}

func TestConfig_UpsertOverwrites(t *testing.T) {
	s := testStore(t)

	_ = s.SetConfig("cloud_url", "http://old.com")
	_ = s.SetConfig("cloud_url", "http://new.com")

	v := s.GetConfig("cloud_url")
	if v != "http://new.com" {
		t.Errorf("got %q, want %q", v, "http://new.com")
	}
}

func TestConfig_UnknownKeyReturnsEmpty(t *testing.T) {
	s := testStore(t)
	v := s.GetConfig("nonexistent_key")
	if v != "" {
		t.Errorf("expected empty for unknown key, got %q", v)
	}
}

func TestSyncState_DefaultEmpty(t *testing.T) {
	s := testStore(t)
	ts := s.GetLastSyncAt("vouchers")
	if ts != "" {
		t.Errorf("expected empty default, got %q", ts)
	}
}

func TestSyncState_SetAndGet(t *testing.T) {
	s := testStore(t)

	err := s.SetLastSyncAt("vouchers", "20240401120000")
	if err != nil {
		t.Fatalf("SetLastSyncAt: %v", err)
	}

	ts := s.GetLastSyncAt("vouchers")
	if ts != "20240401120000" {
		t.Errorf("got %q, want %q", ts, "20240401120000")
	}
}

func TestSyncState_UnknownCollectionErrors(t *testing.T) {
	s := testStore(t)
	err := s.SetLastSyncAt("unknown_collection", "20240101")
	if err == nil {
		t.Error("expected error for unknown collection")
	}
}

func TestSyncState_AllCollectionsSeeded(t *testing.T) {
	s := testStore(t)
	collections := []string{"vouchers", "ledgers", "stock_items"}
	for _, c := range collections {
		// Should not error — all seeded
		err := s.SetLastSyncAt(c, "20240101000000")
		if err != nil {
			t.Errorf("SetLastSyncAt(%q): %v", c, err)
		}
		v := s.GetLastSyncAt(c)
		if v != "20240101000000" {
			t.Errorf("GetLastSyncAt(%q) = %q, want 20240101000000", c, v)
		}
	}
}

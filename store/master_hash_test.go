package store

import (
	"testing"
)

// ── Group A: GetMasterHash / SetMasterHash round-trip ─────────────────────────

func TestMasterHash_SetAndGet_RoundTrip(t *testing.T) {
	s := testStore(t)

	// Not yet set — should return empty string, no error.
	h, err := s.GetMasterHash("ledger", "guid-001")
	if err != nil {
		t.Fatalf("GetMasterHash (not set): %v", err)
	}
	if h != "" {
		t.Errorf("expected empty hash before set, got %q", h)
	}

	// Set hash.
	if err := s.SetMasterHash("ledger", "guid-001", "abc123"); err != nil {
		t.Fatalf("SetMasterHash: %v", err)
	}

	// Get should return the stored value.
	h, err = s.GetMasterHash("ledger", "guid-001")
	if err != nil {
		t.Fatalf("GetMasterHash (after set): %v", err)
	}
	if h != "abc123" {
		t.Errorf("GetMasterHash = %q, want abc123", h)
	}
}

func TestMasterHash_SetOverwrites(t *testing.T) {
	s := testStore(t)

	_ = s.SetMasterHash("stock_item", "si-001", "hash-v1")
	_ = s.SetMasterHash("stock_item", "si-001", "hash-v2")

	h, err := s.GetMasterHash("stock_item", "si-001")
	if err != nil {
		t.Fatalf("GetMasterHash: %v", err)
	}
	if h != "hash-v2" {
		t.Errorf("expected hash-v2 after overwrite, got %q", h)
	}
}

func TestMasterHash_AllEntityTypes(t *testing.T) {
	s := testStore(t)

	types := []string{"ledger", "stock_item", "stock_group", "uom", "godown"}
	for _, et := range types {
		if err := s.SetMasterHash(et, "guid-x", "hash-x"); err != nil {
			t.Errorf("SetMasterHash(%q): %v", et, err)
		}
		h, err := s.GetMasterHash(et, "guid-x")
		if err != nil {
			t.Errorf("GetMasterHash(%q): %v", et, err)
		}
		if h != "hash-x" {
			t.Errorf("GetMasterHash(%q) = %q, want hash-x", et, h)
		}
	}
}

func TestMasterHash_UnknownEntityType_Errors(t *testing.T) {
	s := testStore(t)

	if err := s.SetMasterHash("unknown_type", "guid-x", "hash-x"); err == nil {
		t.Error("expected error for unknown entity type in SetMasterHash, got nil")
	}
	if _, err := s.GetMasterHash("unknown_type", "guid-x"); err == nil {
		t.Error("expected error for unknown entity type in GetMasterHash, got nil")
	}
}

func TestMasterHash_DifferentGuids_StoredSeparately(t *testing.T) {
	s := testStore(t)

	_ = s.SetMasterHash("ledger", "g-001", "hash-a")
	_ = s.SetMasterHash("ledger", "g-002", "hash-b")

	ha, _ := s.GetMasterHash("ledger", "g-001")
	hb, _ := s.GetMasterHash("ledger", "g-002")

	if ha != "hash-a" {
		t.Errorf("g-001 hash = %q, want hash-a", ha)
	}
	if hb != "hash-b" {
		t.Errorf("g-002 hash = %q, want hash-b", hb)
	}
}

func TestMasterHash_DifferentEntityTypes_SameGuid(t *testing.T) {
	s := testStore(t)

	// Same GUID can exist under different entity types independently.
	_ = s.SetMasterHash("ledger", "shared-guid", "hash-ledger")
	_ = s.SetMasterHash("stock_item", "shared-guid", "hash-item")

	hl, _ := s.GetMasterHash("ledger", "shared-guid")
	hi, _ := s.GetMasterHash("stock_item", "shared-guid")

	if hl != "hash-ledger" {
		t.Errorf("ledger hash = %q, want hash-ledger", hl)
	}
	if hi != "hash-item" {
		t.Errorf("stock_item hash = %q, want hash-item", hi)
	}
}

// ── Group B: GetChangedMasters ────────────────────────────────────────────────

func TestGetChangedMasters_AllNew_AllReturned(t *testing.T) {
	s := testStore(t)

	entities := []MasterEntity{
		{GUID: "g-001", Hash: "h1"},
		{GUID: "g-002", Hash: "h2"},
		{GUID: "g-003", Hash: "h3"},
	}

	changed, err := s.GetChangedMasters("ledger", entities)
	if err != nil {
		t.Fatalf("GetChangedMasters: %v", err)
	}
	// All are new — all should be returned.
	if len(changed) != 3 {
		t.Errorf("expected 3 changed GUIDs (all new), got %d", len(changed))
	}
}

func TestGetChangedMasters_UnchangedNotReturned(t *testing.T) {
	s := testStore(t)

	// Pre-seed g-001 with hash "h1" (already synced).
	_ = s.SetMasterHash("ledger", "g-001", "h1")

	entities := []MasterEntity{
		{GUID: "g-001", Hash: "h1"}, // unchanged
		{GUID: "g-002", Hash: "h2"}, // new
	}

	changed, err := s.GetChangedMasters("ledger", entities)
	if err != nil {
		t.Fatalf("GetChangedMasters: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed GUID, got %d: %v", len(changed), changed)
	}
	if changed[0] != "g-002" {
		t.Errorf("changed[0] = %q, want g-002", changed[0])
	}
}

func TestGetChangedMasters_HashChanged_Returned(t *testing.T) {
	s := testStore(t)

	// Pre-seed g-001 with old hash.
	_ = s.SetMasterHash("stock_group", "sg-001", "old-hash")

	entities := []MasterEntity{
		{GUID: "sg-001", Hash: "new-hash"}, // hash changed
	}

	changed, err := s.GetChangedMasters("stock_group", entities)
	if err != nil {
		t.Fatalf("GetChangedMasters: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("expected 1 changed GUID, got %d", len(changed))
	}
	if changed[0] != "sg-001" {
		t.Errorf("changed[0] = %q, want sg-001", changed[0])
	}
}

func TestGetChangedMasters_AllSynced_NoneReturned(t *testing.T) {
	s := testStore(t)

	_ = s.SetMasterHash("uom", "u-001", "hash-u1")
	_ = s.SetMasterHash("uom", "u-002", "hash-u2")

	entities := []MasterEntity{
		{GUID: "u-001", Hash: "hash-u1"}, // unchanged
		{GUID: "u-002", Hash: "hash-u2"}, // unchanged
	}

	changed, err := s.GetChangedMasters("uom", entities)
	if err != nil {
		t.Fatalf("GetChangedMasters: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("expected 0 changed GUIDs (all synced), got %d: %v", len(changed), changed)
	}
}

func TestGetChangedMasters_EmptyInput_ReturnsEmpty(t *testing.T) {
	s := testStore(t)

	changed, err := s.GetChangedMasters("godown", []MasterEntity{})
	if err != nil {
		t.Fatalf("GetChangedMasters: %v", err)
	}
	if len(changed) != 0 {
		t.Errorf("expected 0 changed GUIDs for empty input, got %d", len(changed))
	}
}

func TestGetChangedMasters_UnknownEntityType_Errors(t *testing.T) {
	s := testStore(t)
	_, err := s.GetChangedMasters("bad_type", []MasterEntity{{GUID: "x", Hash: "h"}})
	if err == nil {
		t.Error("expected error for unknown entity type, got nil")
	}
}

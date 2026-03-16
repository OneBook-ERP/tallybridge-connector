package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tallybridge/connector/store"
	"github.com/tallybridge/connector/tally"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func testStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	s, err := store.InitDB(path)
	if err != nil {
		t.Fatalf("store.InitDB: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// tallyServerWith returns a mock Tally server that always responds with xml.
func tallyServerWith(xml string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(xml))
	}))
}

// simpleCloudServer returns a mock cloud server that responds with the given status code.
func simpleCloudServer(statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
}

// voucherXML builds a minimal voucher response with the given flags.
func voucherXML(guid, isCancelled, isDeleted, isOptional string) string {
	return `<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>` + guid + `</GUID>
  <DATE>20230401</DATE>
  <VOUCHERTYPENAME>Journal</VOUCHERTYPENAME>
  <ISCANCELLED>` + isCancelled + `</ISCANCELLED>
  <ISDELETED>` + isDeleted + `</ISDELETED>
  <ISOPTIONAL>` + isOptional + `</ISOPTIONAL>
  <ALLLEDGERENTRIES.LIST>
    <LEDGERNAME>Cash</LEDGERNAME>
    <ISDEEMEDPOSITIVE>Yes</ISDEEMEDPOSITIVE>
    <ISPARTYLEDGER>No</ISPARTYLEDGER>
    <AMOUNT>100.00</AMOUNT>
  </ALLLEDGERENTRIES.LIST>
  <ALLLEDGERENTRIES.LIST>
    <LEDGERNAME>Income</LEDGERNAME>
    <ISDEEMEDPOSITIVE>No</ISDEEMEDPOSITIVE>
    <ISPARTYLEDGER>No</ISPARTYLEDGER>
    <AMOUNT>-100.00</AMOUNT>
  </ALLLEDGERENTRIES.LIST>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`
}

// ── Group A: DeltaSync filtering ──────────────────────────────────────────────

func TestDeltaSync_FiltersOptionalVouchers(t *testing.T) {
	tSrv := tallyServerWith(voucherXML("opt-001", "No", "No", "Yes"))
	defer tSrv.Close()

	cSrv := simpleCloudServer(http.StatusOK)
	defer cSrv.Close()

	db := testStore(t)
	_ = db.SetConfig("sync_from_date", "2023-04-01")
	_ = db.SetConfig("sync_to_date", "2023-04-30")

	client := tally.NewClient(tally.WithAddr(tSrv.URL))
	if err := DeltaSync(context.Background(), client, db, cSrv.URL, "tok", "cid", "Demo Co"); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	// Draft voucher filtered → voucher watermark must NOT be set
	if wm := db.GetLastSyncAt("vouchers"); wm != "" {
		t.Errorf("voucher watermark should be empty (draft filtered), got %q", wm)
	}
}

func TestDeltaSync_FiltersCancelledVouchers(t *testing.T) {
	tSrv := tallyServerWith(voucherXML("can-001", "Yes", "No", "No"))
	defer tSrv.Close()

	cSrv := simpleCloudServer(http.StatusOK)
	defer cSrv.Close()

	db := testStore(t)
	_ = db.SetConfig("sync_from_date", "2023-04-01")
	_ = db.SetConfig("sync_to_date", "2023-04-30")

	client := tally.NewClient(tally.WithAddr(tSrv.URL))
	if err := DeltaSync(context.Background(), client, db, cSrv.URL, "tok", "cid", "Demo Co"); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	if wm := db.GetLastSyncAt("vouchers"); wm != "" {
		t.Errorf("voucher watermark should be empty (cancelled filtered), got %q", wm)
	}
}

func TestDeltaSync_FiltersDeletedVouchers(t *testing.T) {
	tSrv := tallyServerWith(voucherXML("del-001", "No", "Yes", "No"))
	defer tSrv.Close()

	cSrv := simpleCloudServer(http.StatusOK)
	defer cSrv.Close()

	db := testStore(t)
	_ = db.SetConfig("sync_from_date", "2023-04-01")
	_ = db.SetConfig("sync_to_date", "2023-04-30")

	client := tally.NewClient(tally.WithAddr(tSrv.URL))
	if err := DeltaSync(context.Background(), client, db, cSrv.URL, "tok", "cid", "Demo Co"); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	if wm := db.GetLastSyncAt("vouchers"); wm != "" {
		t.Errorf("voucher watermark should be empty (deleted filtered), got %q", wm)
	}
}

// ── Group B: watermark behaviour ─────────────────────────────────────────────

func TestDeltaSync_WatermarkAdvancesOnSuccess(t *testing.T) {
	tSrv := tallyServerWith(voucherXML("ok-001", "No", "No", "No"))
	defer tSrv.Close()

	cSrv := simpleCloudServer(http.StatusOK)
	defer cSrv.Close()

	db := testStore(t)
	_ = db.SetConfig("sync_from_date", "2023-04-01")
	_ = db.SetConfig("sync_to_date", "2023-04-30")

	client := tally.NewClient(tally.WithAddr(tSrv.URL))
	if err := DeltaSync(context.Background(), client, db, cSrv.URL, "tok", "cid", "Demo Co"); err != nil {
		t.Fatalf("DeltaSync: %v", err)
	}
	if wm := db.GetLastSyncAt("vouchers"); wm == "" {
		t.Error("voucher watermark should be set after successful sync")
	}
}

func TestDeltaSync_WatermarkNotAdvancedOnCloudError(t *testing.T) {
	tSrv := tallyServerWith(voucherXML("fail-001", "No", "No", "No"))
	defer tSrv.Close()

	cSrv := simpleCloudServer(http.StatusInternalServerError)
	defer cSrv.Close()

	db := testStore(t)
	_ = db.SetConfig("sync_from_date", "2023-04-01")
	_ = db.SetConfig("sync_to_date", "2023-04-30")

	client := tally.NewClient(tally.WithAddr(tSrv.URL))
	err := DeltaSync(context.Background(), client, db, cSrv.URL, "tok", "cid", "Demo Co")
	if err == nil {
		t.Error("expected error when cloud returns 500")
	}
	if wm := db.GetLastSyncAt("vouchers"); wm != "" {
		t.Errorf("voucher watermark must not advance on cloud error, got %q", wm)
	}
}

// ── Group C: sendBatchToCloud (delta package tests, prefixed Delta to avoid collision) ───────

func TestDeltaSendBatch_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	batch := SyncBatch{CompanyID: "cid", BatchID: "b1", ToTimestamp: "20230401120000"}
	if err := sendBatchToCloud(context.Background(), srv.URL, "secret-xyz", batch); err != nil {
		t.Fatalf("sendBatchToCloud: %v", err)
	}
	if gotAuth != "Bearer secret-xyz" {
		t.Errorf("Authorization = %q, want Bearer secret-xyz", gotAuth)
	}
}

func TestDeltaSendBatch_PostsToSyncDataPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_ = sendBatchToCloud(context.Background(), srv.URL, "tok", SyncBatch{CompanyID: "c1", BatchID: "b1"})
	if !strings.HasSuffix(gotPath, "/connector/sync-data") {
		t.Errorf("path = %q, want /connector/sync-data", gotPath)
	}
}

func TestDeltaSendBatch_Accepts201(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	if err := sendBatchToCloud(context.Background(), srv.URL, "tok", SyncBatch{}); err != nil {
		t.Errorf("expected nil error for 201, got: %v", err)
	}
}

func TestDeltaSendBatch_ErrorOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := sendBatchToCloud(context.Background(), srv.URL, "tok", SyncBatch{}); err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

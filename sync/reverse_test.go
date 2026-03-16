package sync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tallybridge/connector/tally/writer"
)

// ── Group B: toWriterVoucherInput — pure conversion function ─────────────────

func TestToWriterVoucherInput_CreateAction(t *testing.T) {
	in := TallyVoucherInput{
		Action:      "CREATE",
		GUID:        "guid-001",
		VoucherType: "Sales Invoice",
		Date:        "20240401",
		IsInvoice:   true,
	}
	out, err := toWriterVoucherInput(in)
	if err != nil {
		t.Fatalf("toWriterVoucherInput: %v", err)
	}
	if out.Action != writer.ActionCreate {
		t.Errorf("Action: got %q, want %q", out.Action, writer.ActionCreate)
	}
	if out.GUID != "guid-001" {
		t.Errorf("GUID: got %q, want guid-001", out.GUID)
	}
	if out.VoucherType != "Sales Invoice" {
		t.Errorf("VoucherType: got %q, want Sales Invoice", out.VoucherType)
	}
	want := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	if !out.Date.Equal(want) {
		t.Errorf("Date: got %v, want %v", out.Date, want)
	}
	if !out.IsInvoice {
		t.Error("IsInvoice: expected true")
	}
}

func TestToWriterVoucherInput_AlterAction(t *testing.T) {
	in := TallyVoucherInput{Action: "ALTER", GUID: "g", Date: "20240101"}
	out, err := toWriterVoucherInput(in)
	if err != nil {
		t.Fatalf("toWriterVoucherInput: %v", err)
	}
	if out.Action != writer.ActionAlter {
		t.Errorf("Action: got %q, want %q", out.Action, writer.ActionAlter)
	}
}

func TestToWriterVoucherInput_CancelAction(t *testing.T) {
	in := TallyVoucherInput{Action: "CANCEL", GUID: "g", Date: "20240101"}
	out, err := toWriterVoucherInput(in)
	if err != nil {
		t.Fatalf("toWriterVoucherInput: %v", err)
	}
	if out.Action != writer.ActionCancel {
		t.Errorf("Action: got %q, want %q", out.Action, writer.ActionCancel)
	}
}

func TestToWriterVoucherInput_UnknownActionDefaultsToCreate(t *testing.T) {
	// Anything not ALTER/CANCEL should default to Create
	in := TallyVoucherInput{Action: "SOMETHING_ELSE", GUID: "g", Date: "20240101"}
	out, err := toWriterVoucherInput(in)
	if err != nil {
		t.Fatalf("toWriterVoucherInput: %v", err)
	}
	if out.Action != writer.ActionCreate {
		t.Errorf("Action: got %q, want %q (default)", out.Action, writer.ActionCreate)
	}
}

func TestToWriterVoucherInput_InvalidDate(t *testing.T) {
	in := TallyVoucherInput{Action: "CREATE", GUID: "g", Date: "not-a-date"}
	_, err := toWriterVoucherInput(in)
	if err == nil {
		t.Error("expected error for invalid date, got nil")
	}
}

func TestToWriterVoucherInput_LedgerEntries(t *testing.T) {
	in := TallyVoucherInput{
		Action: "CREATE",
		GUID:   "g",
		Date:   "20240401",
		LedgerEntries: []TallyLedgerEntryInput{
			{
				LedgerName:       "B2B Sales",
				IsDeemedPositive: false,
				IsPartyLedger:    false,
				Amount:           -10000,
			},
			{
				LedgerName:       "Customer ABC",
				IsDeemedPositive: true,
				IsPartyLedger:    true,
				Amount:           11800,
			},
		},
	}
	out, err := toWriterVoucherInput(in)
	if err != nil {
		t.Fatalf("toWriterVoucherInput: %v", err)
	}
	if len(out.LedgerEntries) != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", len(out.LedgerEntries))
	}
	if out.LedgerEntries[0].LedgerName != "B2B Sales" {
		t.Errorf("ledger[0].LedgerName: got %q", out.LedgerEntries[0].LedgerName)
	}
	if out.LedgerEntries[0].Amount != -10000 {
		t.Errorf("ledger[0].Amount: got %v, want -10000", out.LedgerEntries[0].Amount)
	}
	if out.LedgerEntries[1].IsPartyLedger != true {
		t.Error("ledger[1].IsPartyLedger: expected true")
	}
}

func TestToWriterVoucherInput_BillAllocations(t *testing.T) {
	in := TallyVoucherInput{
		Action: "CREATE",
		GUID:   "g",
		Date:   "20240401",
		LedgerEntries: []TallyLedgerEntryInput{
			{
				LedgerName: "Customer ABC",
				Amount:     11800,
				BillAllocations: []TallyBillAllocationInput{
					{Name: "SI-001", BillType: "New Ref", Amount: 11800, DueDate: "20240501"},
					{Name: "SI-002", BillType: "Agst Ref", Amount: 5000, DueDate: ""},
				},
			},
		},
	}
	out, err := toWriterVoucherInput(in)
	if err != nil {
		t.Fatalf("toWriterVoucherInput: %v", err)
	}
	bills := out.LedgerEntries[0].BillAllocations
	if len(bills) != 2 {
		t.Fatalf("expected 2 bill allocations, got %d", len(bills))
	}
	if bills[0].Name != "SI-001" {
		t.Errorf("bill[0].Name: got %q, want SI-001", bills[0].Name)
	}
	if bills[0].BillType != "New Ref" {
		t.Errorf("bill[0].BillType: got %q, want New Ref", bills[0].BillType)
	}
	wantDue := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	if !bills[0].DueDate.Equal(wantDue) {
		t.Errorf("bill[0].DueDate: got %v, want %v", bills[0].DueDate, wantDue)
	}
	// Empty DueDate should parse to zero
	if !bills[1].DueDate.IsZero() {
		t.Errorf("bill[1].DueDate should be zero for empty string, got %v", bills[1].DueDate)
	}
}

func TestToWriterVoucherInput_InventoryEntries(t *testing.T) {
	in := TallyVoucherInput{
		Action: "CREATE",
		GUID:   "g",
		Date:   "20240401",
		InventoryEntries: []TallyInventoryEntryInput{
			{
				StockItemName: "Widget A",
				Rate:          100,
				RateUOM:       "Nos",
				Amount:        1000,
				BilledQty:     10,
				BilledQtyUOM:  "Nos",
				HSNCode:       "84719000",
				AccountLedger: "B2B Sales",
			},
		},
	}
	out, err := toWriterVoucherInput(in)
	if err != nil {
		t.Fatalf("toWriterVoucherInput: %v", err)
	}
	if len(out.InventoryEntries) != 1 {
		t.Fatalf("expected 1 inventory entry, got %d", len(out.InventoryEntries))
	}
	ie := out.InventoryEntries[0]
	if ie.StockItemName != "Widget A" {
		t.Errorf("StockItemName: got %q", ie.StockItemName)
	}
	if ie.HSNCode != "84719000" {
		t.Errorf("HSNCode: got %q", ie.HSNCode)
	}
	if ie.AccountLedger != "B2B Sales" {
		t.Errorf("AccountLedger: got %q", ie.AccountLedger)
	}
	if ie.BilledQty != 10 {
		t.Errorf("BilledQty: got %v, want 10", ie.BilledQty)
	}
}

func TestToWriterVoucherInput_AllFields(t *testing.T) {
	in := TallyVoucherInput{
		Action:          "ALTER",
		GUID:            "guid-full",
		VoucherType:     "Purchase Invoice",
		ObjView:         "Invoice Voucher View",
		Date:            "20231201",
		VoucherNumber:   "PI-999",
		ReferenceNumber: "SUP-INV-456",
		PartyLedgerName: "Supplier XYZ",
		Narration:       "Test narration",
		IsInvoice:       true,
		PlaceOfSupply:   "Tamil Nadu",
		IsRCMApplicable: true,
	}
	out, err := toWriterVoucherInput(in)
	if err != nil {
		t.Fatalf("toWriterVoucherInput: %v", err)
	}
	if out.VoucherNumber != "PI-999" {
		t.Errorf("VoucherNumber: got %q", out.VoucherNumber)
	}
	if out.ReferenceNumber != "SUP-INV-456" {
		t.Errorf("ReferenceNumber: got %q", out.ReferenceNumber)
	}
	if out.PartyLedgerName != "Supplier XYZ" {
		t.Errorf("PartyLedgerName: got %q", out.PartyLedgerName)
	}
	if out.Narration != "Test narration" {
		t.Errorf("Narration: got %q", out.Narration)
	}
	if out.PlaceOfSupply != "Tamil Nadu" {
		t.Errorf("PlaceOfSupply: got %q", out.PlaceOfSupply)
	}
	if !out.IsRCMApplicable {
		t.Error("IsRCMApplicable: expected true")
	}
}

// ── Group C: sendBatchToCloud — HTTP transport ───────────────────────────────

func TestSendBatchToCloud_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	batch := SyncBatch{
		CompanyID:     "cmp-001",
		BatchID:       "batch-001",
		FromTimestamp: "",
		ToTimestamp:   "20240401120000",
	}

	err := sendBatchToCloud(t.Context(), server.URL, "my-token", batch)
	if err != nil {
		t.Fatalf("sendBatchToCloud: %v", err)
	}
}

func TestSendBatchToCloud_201Accepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	batch := SyncBatch{CompanyID: "cmp", BatchID: "b", ToTimestamp: "20240101"}
	if err := sendBatchToCloud(t.Context(), server.URL, "", batch); err != nil {
		t.Fatalf("sendBatchToCloud with 201: %v", err)
	}
}

func TestSendBatchToCloud_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer server.Close()

	batch := SyncBatch{CompanyID: "cmp", BatchID: "b", ToTimestamp: "20240101"}
	err := sendBatchToCloud(t.Context(), server.URL, "token", batch)
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestSendBatchToCloud_AuthHeader(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	batch := SyncBatch{CompanyID: "cmp", BatchID: "b", ToTimestamp: "20240101"}
	_ = sendBatchToCloud(t.Context(), server.URL, "secret-token-xyz", batch)

	if gotAuth != "Bearer secret-token-xyz" {
		t.Errorf("Authorization header: got %q, want %q", gotAuth, "Bearer secret-token-xyz")
	}
}

func TestSendBatchToCloud_BatchPayloadFields(t *testing.T) {
	var received SyncBatch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	batch := SyncBatch{
		CompanyID:     "company-abc",
		BatchID:       "batch-xyz",
		FromTimestamp: "20240101000000",
		ToTimestamp:   "20240401000000",
	}
	_ = sendBatchToCloud(t.Context(), server.URL, "", batch)

	if received.CompanyID != "company-abc" {
		t.Errorf("CompanyID: got %q", received.CompanyID)
	}
	if received.BatchID != "batch-xyz" {
		t.Errorf("BatchID: got %q", received.BatchID)
	}
	if received.FromTimestamp != "20240101000000" {
		t.Errorf("FromTimestamp: got %q", received.FromTimestamp)
	}
	if received.ToTimestamp != "20240401000000" {
		t.Errorf("ToTimestamp: got %q", received.ToTimestamp)
	}
}

func TestSendBatchToCloud_PostsToSyncDataEndpoint(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	batch := SyncBatch{CompanyID: "c", BatchID: "b", ToTimestamp: "20240101"}
	_ = sendBatchToCloud(t.Context(), server.URL, "", batch)

	if gotPath != "/connector/sync-data" {
		t.Errorf("path: got %q, want /connector/sync-data", gotPath)
	}
}

func TestSendBatchToCloud_ContentTypeJSON(t *testing.T) {
	var gotCT string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	batch := SyncBatch{CompanyID: "c", BatchID: "b", ToTimestamp: "20240101"}
	_ = sendBatchToCloud(t.Context(), server.URL, "", batch)

	if gotCT != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", gotCT)
	}
}

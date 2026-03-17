package reader

import (
	"context"
	"testing"
	"time"

	"github.com/tallybridge/connector/tally"
)

// ── Group A: parseStockVouchersResponse ───────────────────────────────────────

func TestParseStockVouchers_TwoStockJournals(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>sv-001</GUID>
  <DATE>20230501</DATE>
  <VOUCHERTYPENAME>Stock Journal</VOUCHERTYPENAME>
  <VOUCHERNUMBER>SJ-001</VOUCHERNUMBER>
  <NARRATION>Raw material consumption</NARRATION>
  <ISDELETED>No</ISDELETED>
  <ISCANCELLED>No</ISCANCELLED>
  <ISOPTIONAL>No</ISOPTIONAL>
  <ALLINVENTORYENTRIES.LIST>
    <STOCKITEMNAME>FILLER</STOCKITEMNAME>
    <ACTUALQTY>500.000 KGS</ACTUALQTY>
    <BILLEDQTY>500.000 KGS</BILLEDQTY>
    <RATE>36.50/KGS</RATE>
    <AMOUNT>-18250.00</AMOUNT>
    <BATCHALLOCATIONS.LIST>
      <GODOWNNAME>Main Store</GODOWNNAME>
      <BATCHNAME>B001</BATCHNAME>
      <SOURCEGODOWNNAME>Transit</SOURCEGODOWNNAME>
    </BATCHALLOCATIONS.LIST>
  </ALLINVENTORYENTRIES.LIST>
</VOUCHER>
<VOUCHER>
  <GUID>sv-002</GUID>
  <DATE>20230510</DATE>
  <VOUCHERTYPENAME>Stock Journal</VOUCHERTYPENAME>
  <VOUCHERNUMBER>SJ-002</VOUCHERNUMBER>
  <NARRATION>Finished goods transfer</NARRATION>
  <ISDELETED>No</ISDELETED>
  <ISCANCELLED>No</ISCANCELLED>
  <ISOPTIONAL>No</ISOPTIONAL>
  <ALLINVENTORYENTRIES.LIST>
    <STOCKITEMNAME>PRODUCT A</STOCKITEMNAME>
    <ACTUALQTY>100.000 Nos</ACTUALQTY>
    <BILLEDQTY>100.000 Nos</BILLEDQTY>
    <RATE>250.00/Nos</RATE>
    <AMOUNT>25000.00</AMOUNT>
    <BATCHALLOCATIONS.LIST>
      <GODOWNNAME>Finished Goods</GODOWNNAME>
      <BATCHNAME></BATCHNAME>
      <SOURCEGODOWNNAME></SOURCEGODOWNNAME>
    </BATCHALLOCATIONS.LIST>
  </ALLINVENTORYENTRIES.LIST>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`)

	vouchers, err := parseStockVouchersResponse(raw)
	if err != nil {
		t.Fatalf("parseStockVouchersResponse: %v", err)
	}
	if len(vouchers) != 2 {
		t.Fatalf("expected 2 stock vouchers, got %d", len(vouchers))
	}

	v1 := vouchers[0]
	if v1.GUID != "sv-001" {
		t.Errorf("v1.GUID = %q, want sv-001", v1.GUID)
	}
	if v1.VoucherTypeName != "Stock Journal" {
		t.Errorf("v1.VoucherTypeName = %q, want Stock Journal", v1.VoucherTypeName)
	}
	if v1.VoucherNumber != "SJ-001" {
		t.Errorf("v1.VoucherNumber = %q, want SJ-001", v1.VoucherNumber)
	}
	if v1.Narration != "Raw material consumption" {
		t.Errorf("v1.Narration = %q, want 'Raw material consumption'", v1.Narration)
	}
	if v1.Date.Format("20060102") != "20230501" {
		t.Errorf("v1.Date = %v, want 20230501", v1.Date)
	}
	if v1.IsDeleted || v1.IsCancelled || v1.IsOptional {
		t.Error("v1 flags: expected all false")
	}

	// Inventory entry on v1
	if len(v1.InventoryEntries) != 1 {
		t.Fatalf("expected 1 inventory entry on v1, got %d", len(v1.InventoryEntries))
	}
	ie1 := v1.InventoryEntries[0]
	if ie1.StockItemName != "FILLER" {
		t.Errorf("ie1.StockItemName = %q, want FILLER", ie1.StockItemName)
	}
	if ie1.ActualQty != 500.0 {
		t.Errorf("ie1.ActualQty = %.3f, want 500.000", ie1.ActualQty)
	}
	if ie1.ActualQtyUOM != "KGS" {
		t.Errorf("ie1.ActualQtyUOM = %q, want KGS", ie1.ActualQtyUOM)
	}
	if ie1.Rate != 36.50 {
		t.Errorf("ie1.Rate = %.2f, want 36.50", ie1.Rate)
	}
	if ie1.Amount != -18250.0 {
		t.Errorf("ie1.Amount = %.2f, want -18250.00", ie1.Amount)
	}
	if ie1.GodownName != "Main Store" {
		t.Errorf("ie1.GodownName = %q, want 'Main Store'", ie1.GodownName)
	}
	if ie1.BatchName != "B001" {
		t.Errorf("ie1.BatchName = %q, want B001", ie1.BatchName)
	}
	if ie1.SourceGodownName != "Transit" {
		t.Errorf("ie1.SourceGodownName = %q, want Transit", ie1.SourceGodownName)
	}

	v2 := vouchers[1]
	if v2.GUID != "sv-002" {
		t.Errorf("v2.GUID = %q, want sv-002", v2.GUID)
	}
}

func TestParseStockVouchers_WithLedgerEntries(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>sv-led-001</GUID>
  <DATE>20230601</DATE>
  <VOUCHERTYPENAME>Delivery Note</VOUCHERTYPENAME>
  <VOUCHERNUMBER>DN-001</VOUCHERNUMBER>
  <ISDELETED>No</ISDELETED>
  <ISCANCELLED>No</ISCANCELLED>
  <ISOPTIONAL>No</ISOPTIONAL>
  <ALLLEDGERENTRIES.LIST>
    <LEDGERNAME>Party Account</LEDGERNAME>
    <AMOUNT>-50000.00</AMOUNT>
  </ALLLEDGERENTRIES.LIST>
  <ALLLEDGERENTRIES.LIST>
    <LEDGERNAME>Sales Account</LEDGERNAME>
    <AMOUNT>50000.00</AMOUNT>
  </ALLLEDGERENTRIES.LIST>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`)

	vouchers, err := parseStockVouchersResponse(raw)
	if err != nil {
		t.Fatalf("parseStockVouchersResponse: %v", err)
	}
	if len(vouchers) != 1 {
		t.Fatalf("expected 1 voucher, got %d", len(vouchers))
	}
	v := vouchers[0]
	if len(v.LedgerEntries) != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", len(v.LedgerEntries))
	}
	if v.LedgerEntries[0].LedgerName != "Party Account" {
		t.Errorf("LedgerEntries[0].LedgerName = %q, want 'Party Account'", v.LedgerEntries[0].LedgerName)
	}
	if v.LedgerEntries[0].Amount != -50000.0 {
		t.Errorf("LedgerEntries[0].Amount = %.2f, want -50000.00", v.LedgerEntries[0].Amount)
	}
}

func TestParseStockVouchers_InvalidXML(t *testing.T) {
	_, err := parseStockVouchersResponse([]byte("not xml"))
	if err == nil {
		t.Error("expected error for invalid XML, got nil")
	}
}

func TestParseStockVouchers_EmptyCollection(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION/></DATA></BODY></ENVELOPE>`)
	vouchers, err := parseStockVouchersResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vouchers) != 0 {
		t.Errorf("expected 0 vouchers, got %d", len(vouchers))
	}
}

// ── Group B: monthly chunking (reuses monthlyChunks from vouchers.go) ─────────

func TestStockVouchers_MonthlyChunking_SplitsCorrectly(t *testing.T) {
	// Two-month range should produce 2 chunks.
	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2023, 5, 31, 0, 0, 0, 0, time.UTC)

	chunks := monthlyChunks(from, to)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks for 2-month range, got %d", len(chunks))
	}
	if chunks[0].from.Month() != time.April {
		t.Errorf("chunks[0].from.Month = %v, want April", chunks[0].from.Month())
	}
	if chunks[1].from.Month() != time.May {
		t.Errorf("chunks[1].from.Month = %v, want May", chunks[1].from.Month())
	}
}

func TestStockVouchers_MonthlyChunking_FullYearProduces12(t *testing.T) {
	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

	chunks := monthlyChunks(from, to)
	if len(chunks) != 12 {
		t.Fatalf("expected 12 chunks for full FY, got %d", len(chunks))
	}
}

// ── Group C: FetchStockVouchers via mock server ───────────────────────────────

func TestFetchStockVouchers_MockServer_ReturnsVouchers(t *testing.T) {
	xmlResp := `<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>sv-mock-001</GUID>
  <DATE>20230401</DATE>
  <VOUCHERTYPENAME>Stock Journal</VOUCHERTYPENAME>
  <ISDELETED>No</ISDELETED>
  <ISCANCELLED>No</ISCANCELLED>
  <ISOPTIONAL>No</ISOPTIONAL>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`

	srv := tallyXMLServer(t, xmlResp)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2023, 4, 30, 0, 0, 0, 0, time.UTC)

	vouchers, err := FetchStockVouchers(context.Background(), client, "Demo Co", "", from, to)
	if err != nil {
		t.Fatalf("FetchStockVouchers: %v", err)
	}
	if len(vouchers) != 1 {
		t.Fatalf("expected 1 stock voucher, got %d", len(vouchers))
	}
	if vouchers[0].GUID != "sv-mock-001" {
		t.Errorf("GUID = %q, want sv-mock-001", vouchers[0].GUID)
	}
}

func TestFetchStockVouchers_MockServer_DeduplicatesAcrossChunks(t *testing.T) {
	// Server always returns the same GUID — should be deduplicated to 1 entry.
	xmlResp := `<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>sv-dup-001</GUID>
  <DATE>20230401</DATE>
  <VOUCHERTYPENAME>Physical Stock</VOUCHERTYPENAME>
  <ISDELETED>No</ISDELETED>
  <ISCANCELLED>No</ISCANCELLED>
  <ISOPTIONAL>No</ISOPTIONAL>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`

	srv := tallyXMLServer(t, xmlResp)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	// Two months — server returns same GUID each time
	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2023, 5, 31, 0, 0, 0, 0, time.UTC)

	vouchers, err := FetchStockVouchers(context.Background(), client, "Demo Co", "", from, to)
	if err != nil {
		t.Fatalf("FetchStockVouchers: %v", err)
	}
	if len(vouchers) != 1 {
		t.Errorf("expected 1 deduplicated stock voucher, got %d", len(vouchers))
	}
}

// ── Group D: cancelled / deleted filtering ────────────────────────────────────

func TestParseStockVouchers_BoolFlags_CancelledAndDeleted(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>sv-cancel-001</GUID>
  <VOUCHERTYPENAME>Stock Journal</VOUCHERTYPENAME>
  <ISCANCELLED>Yes</ISCANCELLED>
  <ISDELETED>Yes</ISDELETED>
  <ISOPTIONAL>Yes</ISOPTIONAL>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`)

	vouchers, err := parseStockVouchersResponse(raw)
	if err != nil {
		t.Fatalf("parseStockVouchersResponse: %v", err)
	}
	if len(vouchers) != 1 {
		t.Fatalf("expected 1 voucher (parser returns all), got %d", len(vouchers))
	}
	v := vouchers[0]
	// Parser returns them — callers are responsible for filtering.
	if !v.IsCancelled {
		t.Error("IsCancelled: got false, want true")
	}
	if !v.IsDeleted {
		t.Error("IsDeleted: got false, want true")
	}
	if !v.IsOptional {
		t.Error("IsOptional: got false, want true")
	}
}

package reader

import (
	"context"
	"testing"
	"time"

	"github.com/tallybridge/connector/tally"
)

// ── Group A: parseVouchersResponse ───────────────────────────────────────────

func TestParseVouchers_CoreFields(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>abc-123</GUID>
  <DATE>20230415</DATE>
  <ALTEREDON>20230416</ALTEREDON>
  <VOUCHERTYPENAME>Sales</VOUCHERTYPENAME>
  <VOUCHERNUMBER>42</VOUCHERNUMBER>
  <REFERENCE>INV/23/001</REFERENCE>
  <REFERENCEDATE>20230414</REFERENCEDATE>
  <PARTYLEDGERNAME>FINE PACKAGING</PARTYLEDGERNAME>
  <NARRATION>Test narration</NARRATION>
  <PLACEOFSUPPLY>Tamil Nadu</PLACEOFSUPPLY>
  <PARTYGSTIN>33AHAPJ3412C1Z9</PARTYGSTIN>
  <ISINVOICE>Yes</ISINVOICE>
  <ISOPTIONAL>No</ISOPTIONAL>
  <ISCANCELLED>No</ISCANCELLED>
  <ISDELETED>No</ISDELETED>
  <ISRCMAPPLICABLE>No</ISRCMAPPLICABLE>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`)

	vouchers, err := parseVouchersResponse(raw)
	if err != nil {
		t.Fatalf("parseVouchersResponse: %v", err)
	}
	if len(vouchers) != 1 {
		t.Fatalf("expected 1 voucher, got %d", len(vouchers))
	}
	v := vouchers[0]

	if v.GUID != "abc-123" {
		t.Errorf("GUID = %q, want abc-123", v.GUID)
	}
	if v.Date.Format("20060102") != "20230415" {
		t.Errorf("Date = %v, want 20230415", v.Date)
	}
	if v.AlteredOn.Format("20060102") != "20230416" {
		t.Errorf("AlteredOn = %v, want 20230416", v.AlteredOn)
	}
	if v.VoucherTypeName != "Sales" {
		t.Errorf("VoucherTypeName = %q, want Sales", v.VoucherTypeName)
	}
	if v.VoucherNumber != "42" {
		t.Errorf("VoucherNumber = %q, want 42", v.VoucherNumber)
	}
	if v.ReferenceNumber != "INV/23/001" {
		t.Errorf("ReferenceNumber = %q, want INV/23/001", v.ReferenceNumber)
	}
	if v.ReferenceDate.Format("20060102") != "20230414" {
		t.Errorf("ReferenceDate = %v, want 20230414", v.ReferenceDate)
	}
	if v.PartyLedgerName != "FINE PACKAGING" {
		t.Errorf("PartyLedgerName = %q, want FINE PACKAGING", v.PartyLedgerName)
	}
	if v.Narration != "Test narration" {
		t.Errorf("Narration = %q, want Test narration", v.Narration)
	}
	if v.PlaceOfSupply != "Tamil Nadu" {
		t.Errorf("PlaceOfSupply = %q, want Tamil Nadu", v.PlaceOfSupply)
	}
	if v.PartyGSTIN != "33AHAPJ3412C1Z9" {
		t.Errorf("PartyGSTIN = %q, want 33AHAPJ3412C1Z9", v.PartyGSTIN)
	}
	if !v.IsInvoice {
		t.Error("IsInvoice: got false, want true")
	}
	if v.IsOptional {
		t.Error("IsOptional: got true, want false")
	}
	if v.IsCancelled {
		t.Error("IsCancelled: got true, want false")
	}
	if v.IsDeleted {
		t.Error("IsDeleted: got true, want false")
	}
}

func TestParseVouchers_BooleanFlags_CancelledAndDeleted(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>del-001</GUID>
  <ISCANCELLED>Yes</ISCANCELLED>
  <ISDELETED>Yes</ISDELETED>
  <ISOPTIONAL>Yes</ISOPTIONAL>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`)

	vouchers, err := parseVouchersResponse(raw)
	if err != nil {
		t.Fatalf("parseVouchersResponse: %v", err)
	}
	if len(vouchers) != 1 {
		t.Fatalf("expected 1 voucher, got %d", len(vouchers))
	}
	v := vouchers[0]
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

func TestParseVouchers_LedgerEntries(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>led-001</GUID>
  <ALLLEDGERENTRIES.LIST>
    <LEDGERNAME>FINE PACKAGING</LEDGERNAME>
    <ISDEEMEDPOSITIVE>No</ISDEEMEDPOSITIVE>
    <ISPARTYLEDGER>Yes</ISPARTYLEDGER>
    <AMOUNT>-129210.00</AMOUNT>
  </ALLLEDGERENTRIES.LIST>
  <ALLLEDGERENTRIES.LIST>
    <LEDGERNAME>GST SALES</LEDGERNAME>
    <ISDEEMEDPOSITIVE>Yes</ISDEEMEDPOSITIVE>
    <ISPARTYLEDGER>No</ISPARTYLEDGER>
    <AMOUNT>129210.00</AMOUNT>
  </ALLLEDGERENTRIES.LIST>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`)

	vouchers, err := parseVouchersResponse(raw)
	if err != nil {
		t.Fatalf("parseVouchersResponse: %v", err)
	}
	v := vouchers[0]
	if len(v.LedgerEntries) != 2 {
		t.Fatalf("expected 2 ledger entries, got %d", len(v.LedgerEntries))
	}

	party := v.LedgerEntries[0]
	if party.LedgerName != "FINE PACKAGING" {
		t.Errorf("LedgerName = %q, want FINE PACKAGING", party.LedgerName)
	}
	if party.IsDeemedPositive {
		t.Error("IsDeemedPositive: got true, want false (credit)")
	}
	if !party.IsPartyLedger {
		t.Error("IsPartyLedger: got false, want true")
	}
	if party.Amount != -129210.0 {
		t.Errorf("Amount = %.2f, want -129210.00", party.Amount)
	}

	income := v.LedgerEntries[1]
	if !income.IsDeemedPositive {
		t.Error("IsDeemedPositive: got false, want true (debit)")
	}
	if income.Amount != 129210.0 {
		t.Errorf("Amount = %.2f, want 129210.00", income.Amount)
	}
}

func TestParseVouchers_BillAllocations(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>bill-001</GUID>
  <ALLLEDGERENTRIES.LIST>
    <LEDGERNAME>PARTY A</LEDGERNAME>
    <ISDEEMEDPOSITIVE>No</ISDEEMEDPOSITIVE>
    <ISPARTYLEDGER>Yes</ISPARTYLEDGER>
    <AMOUNT>-50000.00</AMOUNT>
    <BILLALLOCATIONS.LIST>
      <NAME>INV-001</NAME>
      <BILLTYPE>New Ref</BILLTYPE>
      <AMOUNT>-50000.00</AMOUNT>
      <DUEDATE>20230515</DUEDATE>
    </BILLALLOCATIONS.LIST>
  </ALLLEDGERENTRIES.LIST>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`)

	vouchers, err := parseVouchersResponse(raw)
	if err != nil {
		t.Fatalf("parseVouchersResponse: %v", err)
	}
	le := vouchers[0].LedgerEntries[0]
	if len(le.BillAllocations) != 1 {
		t.Fatalf("expected 1 bill allocation, got %d", len(le.BillAllocations))
	}
	ba := le.BillAllocations[0]
	if ba.Name != "INV-001" {
		t.Errorf("Name = %q, want INV-001", ba.Name)
	}
	if ba.BillType != "New Ref" {
		t.Errorf("BillType = %q, want New Ref", ba.BillType)
	}
	if ba.Amount != -50000.0 {
		t.Errorf("Amount = %.2f, want -50000.00", ba.Amount)
	}
	if ba.DueDate.Format("20060102") != "20230515" {
		t.Errorf("DueDate = %v, want 20230515", ba.DueDate)
	}
}

func TestParseVouchers_BankAllocations(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>bank-001</GUID>
  <ALLLEDGERENTRIES.LIST>
    <LEDGERNAME>HDFC Bank</LEDGERNAME>
    <ISDEEMEDPOSITIVE>No</ISDEEMEDPOSITIVE>
    <ISPARTYLEDGER>No</ISPARTYLEDGER>
    <AMOUNT>-33000.00</AMOUNT>
    <BANKALLOCATIONS.LIST>
      <INSTNUMBER>123456</INSTNUMBER>
      <UNIQUEREFERENCENUMBER>5bbzKNnq08tMfcBi</UNIQUEREFERENCENUMBER>
      <TRANSACTIONTYPE>NEFT</TRANSACTIONTYPE>
    </BANKALLOCATIONS.LIST>
  </ALLLEDGERENTRIES.LIST>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`)

	vouchers, err := parseVouchersResponse(raw)
	if err != nil {
		t.Fatalf("parseVouchersResponse: %v", err)
	}
	le := vouchers[0].LedgerEntries[0]
	if len(le.BankAllocations) != 1 {
		t.Fatalf("expected 1 bank allocation, got %d", len(le.BankAllocations))
	}
	bk := le.BankAllocations[0]
	if bk.InstNumber != "123456" {
		t.Errorf("InstNumber = %q, want 123456", bk.InstNumber)
	}
	if bk.UniqueReferenceNumber != "5bbzKNnq08tMfcBi" {
		t.Errorf("UniqueReferenceNumber = %q, want 5bbzKNnq08tMfcBi", bk.UniqueReferenceNumber)
	}
	if bk.TransactionType != "NEFT" {
		t.Errorf("TransactionType = %q, want NEFT", bk.TransactionType)
	}
}

func TestParseVouchers_InventoryEntries(t *testing.T) {
	raw := []byte(`<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>inv-001</GUID>
  <ALLINVENTORYENTRIES.LIST>
    <STOCKITEMNAME>FILLER</STOCKITEMNAME>
    <RATE>36.50/KGS</RATE>
    <AMOUNT>109500.00</AMOUNT>
    <ACTUALQTY>3000.000 KGS</ACTUALQTY>
    <BILLEDQTY>3000.000 KGS</BILLEDQTY>
    <GSTHSNNAME>38249900</GSTHSNNAME>
    <GSTOVRDNRATE>18</GSTOVRDNRATE>
    <BATCHALLOCATIONS.LIST>
      <GODOWNNAME>Main Location</GODOWNNAME>
      <EXPIRYDATE></EXPIRYDATE>
    </BATCHALLOCATIONS.LIST>
    <ACCOUNTINGALLOCATIONS.LIST>
      <LEDGERNAME>GST SALES</LEDGERNAME>
    </ACCOUNTINGALLOCATIONS.LIST>
  </ALLINVENTORYENTRIES.LIST>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`)

	vouchers, err := parseVouchersResponse(raw)
	if err != nil {
		t.Fatalf("parseVouchersResponse: %v", err)
	}
	if len(vouchers[0].InventoryEntries) != 1 {
		t.Fatalf("expected 1 inventory entry, got %d", len(vouchers[0].InventoryEntries))
	}
	ie := vouchers[0].InventoryEntries[0]
	if ie.StockItemName != "FILLER" {
		t.Errorf("StockItemName = %q, want FILLER", ie.StockItemName)
	}
	if ie.Rate != 36.50 {
		t.Errorf("Rate = %.2f, want 36.50", ie.Rate)
	}
	if ie.RateUOM != "KGS" {
		t.Errorf("RateUOM = %q, want KGS", ie.RateUOM)
	}
	if ie.BilledQty != 3000.0 {
		t.Errorf("BilledQty = %.3f, want 3000.000", ie.BilledQty)
	}
	if ie.BilledQtyUOM != "KGS" {
		t.Errorf("BilledQtyUOM = %q, want KGS", ie.BilledQtyUOM)
	}
	if ie.Amount != 109500.0 {
		t.Errorf("Amount = %.2f, want 109500.00", ie.Amount)
	}
	if ie.HSNCode != "38249900" {
		t.Errorf("HSNCode = %q, want 38249900", ie.HSNCode)
	}
	if ie.GSTOvrdnRate != 18.0 {
		t.Errorf("GSTOvrdnRate = %.0f, want 18", ie.GSTOvrdnRate)
	}
	if ie.GodownName != "Main Location" {
		t.Errorf("GodownName = %q, want Main Location", ie.GodownName)
	}
	if ie.AccountLedgerName != "GST SALES" {
		t.Errorf("AccountLedgerName = %q, want GST SALES", ie.AccountLedgerName)
	}
}

func TestParseVouchers_InvalidXML(t *testing.T) {
	_, err := parseVouchersResponse([]byte("not xml"))
	if err == nil {
		t.Error("expected error for invalid XML, got nil")
	}
}

// ── Group B: monthlyChunks ────────────────────────────────────────────────────

func TestMonthlyChunks_FullYear(t *testing.T) {
	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

	chunks := monthlyChunks(from, to)
	if len(chunks) != 12 {
		t.Fatalf("expected 12 monthly chunks for FY 2023-24, got %d", len(chunks))
	}
	// First chunk: Apr 2023
	if chunks[0].from.Month() != time.April || chunks[0].from.Year() != 2023 {
		t.Errorf("first chunk from = %v, want 2023-04-01", chunks[0].from)
	}
	// Last chunk: Mar 2024
	if chunks[11].from.Month() != time.March || chunks[11].from.Year() != 2024 {
		t.Errorf("last chunk from = %v, want 2024-03-xx", chunks[11].from)
	}
}

func TestMonthlyChunks_SingleMonth(t *testing.T) {
	from := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2023, 6, 30, 0, 0, 0, 0, time.UTC)

	chunks := monthlyChunks(from, to)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for single month, got %d", len(chunks))
	}
	if chunks[0].from != from {
		t.Errorf("chunk from = %v, want %v", chunks[0].from, from)
	}
	if chunks[0].to != to {
		t.Errorf("chunk to = %v, want %v", chunks[0].to, to)
	}
}

// ── Group C: ListDeltaVouchers via mock server ────────────────────────────────

func TestListDeltaVouchers_MockServer(t *testing.T) {
	xml := `<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>v-001</GUID>
  <DATE>20230401</DATE>
  <VOUCHERTYPENAME>Sales</VOUCHERTYPENAME>
  <ISCANCELLED>No</ISCANCELLED>
  <ISDELETED>No</ISDELETED>
  <ISOPTIONAL>No</ISOPTIONAL>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`

	srv := tallyXMLServer(t, xml)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2023, 4, 30, 0, 0, 0, 0, time.UTC)

	vouchers, err := ListDeltaVouchers(context.Background(), client, "Demo Co", "", from, to)
	if err != nil {
		t.Fatalf("ListDeltaVouchers: %v", err)
	}
	if len(vouchers) != 1 {
		t.Fatalf("expected 1 voucher, got %d", len(vouchers))
	}
	if vouchers[0].GUID != "v-001" {
		t.Errorf("GUID = %q, want v-001", vouchers[0].GUID)
	}
}

func TestListDeltaVouchers_DeduplicatesAcrossChunks(t *testing.T) {
	// Server always returns the same GUID — should be deduplicated to 1 entry.
	xml := `<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER>
  <GUID>dup-guid</GUID>
  <DATE>20230401</DATE>
  <VOUCHERTYPENAME>Journal</VOUCHERTYPENAME>
  <ISCANCELLED>No</ISCANCELLED>
  <ISDELETED>No</ISDELETED>
  <ISOPTIONAL>No</ISOPTIONAL>
</VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`

	srv := tallyXMLServer(t, xml)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	// Two months — server returns same GUID each time
	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2023, 5, 31, 0, 0, 0, 0, time.UTC)

	vouchers, err := ListDeltaVouchers(context.Background(), client, "Demo Co", "", from, to)
	if err != nil {
		t.Fatalf("ListDeltaVouchers: %v", err)
	}
	if len(vouchers) != 1 {
		t.Errorf("expected 1 deduplicated voucher, got %d", len(vouchers))
	}
}

// ── Group D: CountVouchers via mock server ────────────────────────────────────

func TestCountVouchers_MockServer(t *testing.T) {
	// Three GUIDs in response = count of 3
	xml := `<ENVELOPE><BODY><DATA><COLLECTION>
<VOUCHER><GUID>g1</GUID></VOUCHER>
<VOUCHER><GUID>g2</GUID></VOUCHER>
<VOUCHER><GUID>g3</GUID></VOUCHER>
</COLLECTION></DATA></BODY></ENVELOPE>`

	srv := tallyXMLServer(t, xml)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

	count, err := CountVouchers(context.Background(), client, "Demo Co", from, to)
	if err != nil {
		t.Fatalf("CountVouchers: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestCountVouchers_Empty(t *testing.T) {
	xml := `<ENVELOPE><BODY><DATA><COLLECTION/></DATA></BODY></ENVELOPE>`

	srv := tallyXMLServer(t, xml)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

	count, err := CountVouchers(context.Background(), client, "Demo Co", from, to)
	if err != nil {
		t.Fatalf("CountVouchers: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}


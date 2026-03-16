package reader

import (
	"context"
	"testing"
	"time"

	"github.com/tallybridge/connector/tally"
)

// ── Group A: ListStockItems via mock server ───────────────────────────────────

func TestListStockItems_CoreFields(t *testing.T) {
	srv := tallyXMLServer(t, `<ENVELOPE><BODY><DATA><COLLECTION>
<STOCKITEM NAME="FILLER">
  <PARENT>Polymers</PARENT>
  <BASEUNITS>KGS</BASEUNITS>
  <GUID>si-001</GUID>
  <GSTAPPLICABLE>Applicable</GSTAPPLICABLE>
  <TAXCLASSIFICATIONNAME>GST18</TAXCLASSIFICATIONNAME>
  <GSTRATE>18</GSTRATE>
  <OPENINGBALANCE>1000.000 KGS</OPENINGBALANCE>
  <OPENINGRATE>36.50/KGS</OPENINGRATE>
  <OPENINGVALUE>-36500.00</OPENINGVALUE>
  <ALTEREDON>20230401</ALTEREDON>
  <HSNDETAILS.LIST>
    <HSNCODE>38249900</HSNCODE>
  </HSNDETAILS.LIST>
</STOCKITEM>
</COLLECTION></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

	client := tally.NewClient(tally.WithAddr(srv.URL))
	items, err := ListStockItems(context.Background(), client, "Demo Co", from, to)
	if err != nil {
		t.Fatalf("ListStockItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]

	if it.Name != "FILLER" {
		t.Errorf("Name = %q, want FILLER", it.Name)
	}
	if it.Parent != "Polymers" {
		t.Errorf("Parent = %q, want Polymers", it.Parent)
	}
	if it.BaseUnits != "KGS" {
		t.Errorf("BaseUnits = %q, want KGS", it.BaseUnits)
	}
	if it.GUID != "si-001" {
		t.Errorf("GUID = %q, want si-001", it.GUID)
	}
	if it.GSTRate != 18.0 {
		t.Errorf("GSTRate = %.1f, want 18.0", it.GSTRate)
	}
	if it.OpeningBalance != 1000.0 {
		t.Errorf("OpeningBalance = %.3f, want 1000.000", it.OpeningBalance)
	}
	if it.OpeningRate != 36.50 {
		t.Errorf("OpeningRate = %.2f, want 36.50", it.OpeningRate)
	}
	if it.OpeningValue != 36500.0 {
		t.Errorf("OpeningValue = %.2f, want 36500.00", it.OpeningValue)
	}
}

func TestListStockItems_HSN_PrefersHSNDetailsList(t *testing.T) {
	// HSNDETAILS.LIST should win over top-level HSNCODE (which is usually empty)
	srv := tallyXMLServer(t, `<ENVELOPE><BODY><DATA><COLLECTION>
<STOCKITEM NAME="ITEM A">
  <PARENT>Primary</PARENT>
  <GUID>si-hsn-001</GUID>
  <HSNCODE></HSNCODE>
  <HSNDETAILS.LIST>
    <HSNCODE>39201000</HSNCODE>
  </HSNDETAILS.LIST>
</STOCKITEM>
</COLLECTION></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

	client := tally.NewClient(tally.WithAddr(srv.URL))
	items, err := ListStockItems(context.Background(), client, "Demo Co", from, to)
	if err != nil {
		t.Fatalf("ListStockItems: %v", err)
	}
	if items[0].HSNCode != "39201000" {
		t.Errorf("HSNCode = %q, want 39201000 (from HSNDETAILS.LIST)", items[0].HSNCode)
	}
}

func TestListStockItems_OpeningValue_DerivedFromQtyAndRate(t *testing.T) {
	// If OpeningValue is 0 but qty + rate are given, derive value = qty * rate
	srv := tallyXMLServer(t, `<ENVELOPE><BODY><DATA><COLLECTION>
<STOCKITEM NAME="ITEM B">
  <PARENT>Primary</PARENT>
  <GUID>si-deriv-001</GUID>
  <OPENINGBALANCE>100.000 Nos</OPENINGBALANCE>
  <OPENINGRATE>50.00/Nos</OPENINGRATE>
  <OPENINGVALUE>0.00</OPENINGVALUE>
</STOCKITEM>
</COLLECTION></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

	client := tally.NewClient(tally.WithAddr(srv.URL))
	items, err := ListStockItems(context.Background(), client, "Demo Co", from, to)
	if err != nil {
		t.Fatalf("ListStockItems: %v", err)
	}
	if items[0].OpeningValue != 5000.0 {
		t.Errorf("OpeningValue = %.2f, want 5000.00 (derived from 100 * 50)", items[0].OpeningValue)
	}
}

func TestListStockItems_OpeningRate_DerivedFromValueAndQty(t *testing.T) {
	// If OpeningRate is 0 but value + qty are given, derive rate = value / qty
	srv := tallyXMLServer(t, `<ENVELOPE><BODY><DATA><COLLECTION>
<STOCKITEM NAME="ITEM C">
  <PARENT>Primary</PARENT>
  <GUID>si-deriv-002</GUID>
  <OPENINGBALANCE>200.000 Nos</OPENINGBALANCE>
  <OPENINGRATE>0.00/Nos</OPENINGRATE>
  <OPENINGVALUE>-10000.00</OPENINGVALUE>
</STOCKITEM>
</COLLECTION></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	from := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC)

	client := tally.NewClient(tally.WithAddr(srv.URL))
	items, err := ListStockItems(context.Background(), client, "Demo Co", from, to)
	if err != nil {
		t.Fatalf("ListStockItems: %v", err)
	}
	if items[0].OpeningRate != 50.0 {
		t.Errorf("OpeningRate = %.2f, want 50.00 (derived from 10000 / 200)", items[0].OpeningRate)
	}
}

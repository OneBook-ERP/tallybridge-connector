package reader

import (
	"context"
	"testing"

	"github.com/tallybridge/connector/tally"
)

// ── Group A: ListLedgers via mock server ──────────────────────────────────────

func TestListLedgers_CoreFields(t *testing.T) {
	srv := tallyXMLServer(t, `<ENVELOPE><BODY><DATA><COLLECTION>
<LEDGER NAME="FINE PACKAGING">
  <PARENT>Sundry Debtors</PARENT>
  <GUID>abc-led-001</GUID>
  <MAILINGNAME>Fine Packaging Pvt Ltd</MAILINGNAME>
  <PINCODE>641062</PINCODE>
  <COUNTRYNAME>India</COUNTRYNAME>
  <LEDSTATENAME>Tamil Nadu</LEDSTATENAME>
  <EMAIL>billing@fine.com</EMAIL>
  <LEDGERMOBILE>9876543210</LEDGERMOBILE>
  <LEDGERPHONE>04224445555</LEDGERPHONE>
  <PANITNO>ABCDE1234F</PANITNO>
  <GSTREGISTRATIONTYPE>Regular</GSTREGISTRATIONTYPE>
  <PARTYGSTIN>33AHAPJ3412C1Z9</PARTYGSTIN>
  <ISBILLWISEON>Yes</ISBILLWISEON>
  <CREDITPERIOD>30 Days</CREDITPERIOD>
  <BANKACCOUNTNUMBER>123456789012</BANKACCOUNTNUMBER>
  <IFSCODE>HDFC0001234</IFSCODE>
  <ALTEREDON>20230416</ALTEREDON>
</LEDGER>
</COLLECTION></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	ledgers, err := ListLedgers(context.Background(), client, "Demo Co")
	if err != nil {
		t.Fatalf("ListLedgers: %v", err)
	}
	if len(ledgers) != 1 {
		t.Fatalf("expected 1 ledger, got %d", len(ledgers))
	}
	l := ledgers[0]

	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"Name", l.Name, "FINE PACKAGING"},
		{"Parent", l.Parent, "Sundry Debtors"},
		{"GUID", l.GUID, "abc-led-001"},
		{"MailingName", l.MailingName, "Fine Packaging Pvt Ltd"},
		{"PinCode", l.PinCode, "641062"},
		{"CountryName", l.CountryName, "India"},
		{"StateName", l.StateName, "Tamil Nadu"},
		{"Email", l.Email, "billing@fine.com"},
		{"Mobile", l.Mobile, "9876543210"},
		{"Phone", l.Phone, "04224445555"},
		{"PanItNo", l.PanItNo, "ABCDE1234F"},
		{"GSTRegistrationType", l.GSTRegistrationType, "Regular"},
		{"PartyGSTIN", l.PartyGSTIN, "33AHAPJ3412C1Z9"},
		{"CreditPeriod", l.CreditPeriod, "30 Days"},
		{"BankAccountNumber", l.BankAccountNumber, "123456789012"},
		{"IFSCode", l.IFSCode, "HDFC0001234"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
	if !l.IsBillWiseOn {
		t.Error("IsBillWiseOn: got false, want true")
	}
	if l.AlteredOn.Format("20060102") != "20230416" {
		t.Errorf("AlteredOn = %v, want 20230416", l.AlteredOn)
	}
}

func TestListLedgers_MultiLineAddress(t *testing.T) {
	srv := tallyXMLServer(t, `<ENVELOPE><BODY><DATA><COLLECTION>
<LEDGER NAME="PARTY B">
  <PARENT>Sundry Creditors</PARENT>
  <GUID>led-addr-001</GUID>
  <ADDRESS.LIST>
    <ADDRESS>123 Industrial Area</ADDRESS>
    <ADDRESS>Phase II</ADDRESS>
    <ADDRESS>Coimbatore</ADDRESS>
  </ADDRESS.LIST>
</LEDGER>
</COLLECTION></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	ledgers, err := ListLedgers(context.Background(), client, "Demo Co")
	if err != nil {
		t.Fatalf("ListLedgers: %v", err)
	}
	l := ledgers[0]
	if len(l.Address) != 3 {
		t.Fatalf("expected 3 address lines, got %d: %v", len(l.Address), l.Address)
	}
	if l.Address[0] != "123 Industrial Area" {
		t.Errorf("Address[0] = %q, want 123 Industrial Area", l.Address[0])
	}
	if l.Address[2] != "Coimbatore" {
		t.Errorf("Address[2] = %q, want Coimbatore", l.Address[2])
	}
}

func TestListLedgers_OpeningBalance_DebitPositive_CreditNegative(t *testing.T) {
	srv := tallyXMLServer(t, `<ENVELOPE><BODY><DATA><COLLECTION>
<LEDGER NAME="DEBTOR A">
  <PARENT>Sundry Debtors</PARENT>
  <GUID>led-ob-001</GUID>
  <OPENINGBALANCE>50000.00</OPENINGBALANCE>
</LEDGER>
<LEDGER NAME="CREDITOR B">
  <PARENT>Sundry Creditors</PARENT>
  <GUID>led-ob-002</GUID>
  <OPENINGBALANCE>-25000.00</OPENINGBALANCE>
</LEDGER>
</COLLECTION></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	ledgers, err := ListLedgers(context.Background(), client, "Demo Co")
	if err != nil {
		t.Fatalf("ListLedgers: %v", err)
	}
	if len(ledgers) != 2 {
		t.Fatalf("expected 2 ledgers, got %d", len(ledgers))
	}
	if ledgers[0].OpeningBalance != 50000.0 {
		t.Errorf("OpeningBalance (debit) = %.2f, want 50000.00", ledgers[0].OpeningBalance)
	}
	if ledgers[1].OpeningBalance != -25000.0 {
		t.Errorf("OpeningBalance (credit) = %.2f, want -25000.00", ledgers[1].OpeningBalance)
	}
}

func TestListLedgers_Empty(t *testing.T) {
	srv := tallyXMLServer(t, `<ENVELOPE><BODY><DATA><COLLECTION/></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	ledgers, err := ListLedgers(context.Background(), client, "Demo Co")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ledgers) != 0 {
		t.Errorf("expected 0 ledgers, got %d", len(ledgers))
	}
}

func TestListLedgers_MultipleEntries(t *testing.T) {
	srv := tallyXMLServer(t, `<ENVELOPE><BODY><DATA><COLLECTION>
<LEDGER NAME="CASH">
  <PARENT>Cash-in-Hand</PARENT>
  <GUID>led-cash-001</GUID>
</LEDGER>
<LEDGER NAME="SALES ACCOUNT">
  <PARENT>Sales Accounts</PARENT>
  <GUID>led-sales-001</GUID>
</LEDGER>
<LEDGER NAME="OUTPUT CGST">
  <PARENT>Duties &amp; Taxes</PARENT>
  <GUID>led-cgst-001</GUID>
</LEDGER>
</COLLECTION></DATA></BODY></ENVELOPE>`)
	defer srv.Close()

	client := tally.NewClient(tally.WithAddr(srv.URL))
	ledgers, err := ListLedgers(context.Background(), client, "Demo Co")
	if err != nil {
		t.Fatalf("ListLedgers: %v", err)
	}
	if len(ledgers) != 3 {
		t.Fatalf("expected 3 ledgers, got %d", len(ledgers))
	}
	if ledgers[0].Name != "CASH" {
		t.Errorf("ledgers[0].Name = %q, want CASH", ledgers[0].Name)
	}
	if ledgers[2].Name != "OUTPUT CGST" {
		t.Errorf("ledgers[2].Name = %q, want OUTPUT CGST", ledgers[2].Name)
	}
}

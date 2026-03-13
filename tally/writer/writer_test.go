package writer

import (
	"strings"
	"testing"
	"time"
)

func TestBuildVoucher_SalesInvoice(t *testing.T) {
	v := VoucherInput{
		Action:          ActionCreate,
		GUID:            "tb-sales-001",
		VoucherType:     "Sales",
		Date:            time.Date(2024, 4, 10, 0, 0, 0, 0, time.UTC),
		VoucherNumber:   "INV-001",
		PartyLedgerName: "ACME Corp",
		IsInvoice:       true,
		PlaceOfSupply:   "Maharashtra",
		CompanyName:     "Demo Company",
		LedgerEntries: []LedgerEntryInput{
			{LedgerName: "ACME Corp", IsDeemedPositive: true, IsPartyLedger: true, Amount: 11800.00},
			{LedgerName: "Sales Account", IsDeemedPositive: false, Amount: -10000.00},
			{LedgerName: "Output CGST 9%", IsDeemedPositive: false, Amount: -900.00},
			{LedgerName: "Output SGST 9%", IsDeemedPositive: false, Amount: -900.00},
		},
	}

	xml, err := BuildVoucher(v)
	if err != nil {
		t.Fatalf("BuildVoucher: unexpected error: %v", err)
	}

	// Uses "Import Data" format (not old "Import" format)
	assertContains(t, xml, "<TALLYREQUEST>Import Data</TALLYREQUEST>")
	assertContains(t, xml, `ACTION="Create"`)
	// Always uses Accounting Voucher View for imports
	assertContains(t, xml, `OBJVIEW="Accounting Voucher View"`)
	assertContains(t, xml, "<GUID>tb-sales-001</GUID>")
	assertContains(t, xml, "<DATE>20240410</DATE>")
	assertContains(t, xml, "<PARTYLEDGERNAME>ACME Corp</PARTYLEDGERNAME>")
	assertContains(t, xml, "<PLACEOFSUPPLY>Maharashtra</PLACEOFSUPPLY>")
	assertContains(t, xml, "<LEDGERNAME>Output CGST 9%</LEDGERNAME>")
	assertContains(t, xml, "<SVCURRENTCOMPANY>Demo Company</SVCURRENTCOMPANY>")
}

func TestBuildVoucher_Imbalanced(t *testing.T) {
	v := VoucherInput{
		Action:      ActionCreate,
		GUID:        "tb-bad-001",
		VoucherType: "Sales",
		Date:        time.Date(2024, 4, 10, 0, 0, 0, 0, time.UTC),
		IsInvoice:   false,
		LedgerEntries: []LedgerEntryInput{
			{LedgerName: "Party", Amount: 1000.00},
			{LedgerName: "Sales", Amount: -900.00},
		},
	}
	_, err := BuildVoucher(v)
	if err == nil {
		t.Error("expected error for imbalanced voucher, got nil")
	}
	if !strings.Contains(err.Error(), "imbalanced") {
		t.Errorf("error message should say imbalanced, got: %v", err)
	}
}

func TestBuildVoucher_Alter_HasTagAttributes(t *testing.T) {
	v := VoucherInput{
		Action:      ActionAlter,
		GUID:        "existing-guid-xyz",
		VoucherType: "Journal",
		Date:        time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
		LedgerEntries: []LedgerEntryInput{
			{LedgerName: "Cash", Amount: 500.0},
			{LedgerName: "Bank", Amount: -500.0},
		},
	}
	xml, err := BuildVoucher(v)
	if err != nil {
		t.Fatalf("BuildVoucher Alter: %v", err)
	}
	assertContains(t, xml, `ACTION="Alter"`)
	assertContains(t, xml, `TAGNAME="GUID"`)
	assertContains(t, xml, `TAGVALUE="existing-guid-xyz"`)
}

func TestBuildVoucher_Cancel(t *testing.T) {
	v := VoucherInput{
		Action:      ActionCancel,
		GUID:        "cancel-me-guid",
		VoucherType: "Sales",
		Date:        time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
	}
	xml, err := BuildVoucher(v)
	if err != nil {
		t.Fatalf("BuildVoucher Cancel: %v", err)
	}
	assertContains(t, xml, `ACTION="Cancel"`)
	assertContains(t, xml, `TAGVALUE="cancel-me-guid"`)
}

func TestBuildVoucher_BillAllocations(t *testing.T) {
	v := VoucherInput{
		Action:          ActionCreate,
		GUID:            "pay-001",
		VoucherType:     "Payment",
		ObjView:         "Accounting Voucher View",
		Date:            time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		PartyLedgerName: "Vendor ABC",
		LedgerEntries: []LedgerEntryInput{
			{
				LedgerName:       "HDFC Bank",
				IsDeemedPositive: false,
				Amount:           -50000.00,
				BankAllocations: []BankAllocationInput{
					{UniqueReferenceNumber: "UTR123456789", TransactionType: "NEFT"},
				},
			},
			{
				LedgerName:       "Vendor ABC",
				IsDeemedPositive: true,
				IsPartyLedger:    true,
				Amount:           50000.00,
				BillAllocations: []BillAllocationInput{
					{Name: "INV-VDR-001", BillType: "Agst Ref", Amount: 50000.00},
				},
			},
		},
	}
	xml, err := BuildVoucher(v)
	if err != nil {
		t.Fatalf("BuildVoucher Payment: %v", err)
	}
	assertContains(t, xml, "<BILLTYPE>Agst Ref</BILLTYPE>")
	assertContains(t, xml, "<UNIQUEREFERENCENUMBER>UTR123456789</UNIQUEREFERENCENUMBER>")
	assertContains(t, xml, "<TRANSACTIONTYPE>NEFT</TRANSACTIONTYPE>")
}

func TestBuildLedger(t *testing.T) {
	l := LedgerInput{
		Action:              ActionCreate,
		Name:                "Ratan Enterprises",
		Parent:              "Sundry Debtors",
		GUID:                "led-001",
		MailingName:         "Ratan Enterprises Pvt Ltd",
		Address:             []string{"123 MG Road", "Pune"},
		PinCode:             "411001",
		StateName:           "Maharashtra",
		GSTRegistrationType: "Regular",
		PartyGSTIN:          "27AABCR1234D1Z5",
		IsBillWiseOn:        true,
	}
	xml := BuildLedger(l)
	assertContains(t, xml, "<TALLYREQUEST>Import Data</TALLYREQUEST>")
	assertContains(t, xml, "<NAME>Ratan Enterprises</NAME>")
	assertContains(t, xml, "<PARENT>Sundry Debtors</PARENT>")
	assertContains(t, xml, "<ADDR>123 MG Road</ADDR>")
	assertContains(t, xml, "<ADDR>Pune</ADDR>")
	assertContains(t, xml, "<PARTYGSTIN>27AABCR1234D1Z5</PARTYGSTIN>")
	assertContains(t, xml, "<ISBILLWISEON>Yes</ISBILLWISEON>")
}

func TestBuildStockItem(t *testing.T) {
	si := StockItemInput{
		Action:                ActionCreate,
		Name:                  "Laptop Core i7",
		Parent:                "Electronics",
		BaseUnits:             "Nos",
		GUID:                  "si-001",
		HSNCode:               "84715000",
		GSTApplicable:         "Applicable",
		TaxClassificationName: "GST @ 18%",
	}
	xml := BuildStockItem(si)
	assertContains(t, xml, "<TALLYREQUEST>Import Data</TALLYREQUEST>")
	assertContains(t, xml, "<NAME>Laptop Core i7</NAME>")
	assertContains(t, xml, "<HSNCODE>84715000</HSNCODE>")
	assertContains(t, xml, "<TAXCLASSIFICATIONNAME>GST @ 18%</TAXCLASSIFICATIONNAME>")
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected XML to contain %q", needle)
	}
}

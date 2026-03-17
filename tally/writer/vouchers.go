// Package writer builds Tally import XML for write operations (APIs 12-15).
// All functions return a fully-formed XML string ready to POST to port 9000.
// The caller must check the response via parser.ParseTallyError before treating
// a write as successful.
package writer

import (
	"fmt"
	"strings"
	"time"

	"github.com/tallybridge/connector/tally"
)

// ---- Shared types -----------------------------------------------------------

// WriteAction is the Tally import action for a voucher or master.
type WriteAction string

const (
	ActionCreate WriteAction = "Create"
	ActionAlter  WriteAction = "Alter"
	ActionCancel WriteAction = "Cancel"
)

// CostCentreAllocInput is one entry in COSTCENTREALLOCATIONS.LIST for a ledger line.
type CostCentreAllocInput struct {
	CostCentreName string
	Amount         float64
}

// LedgerEntryInput is one line in the ledger entries of an import voucher.
type LedgerEntryInput struct {
	LedgerName            string
	IsDeemedPositive      bool    // true=Debit, false=Credit
	IsPartyLedger         bool
	Amount                float64 // signed; positive=Debit in Tally's internal sense
	BillAllocations       []BillAllocationInput
	BankAllocations       []BankAllocationInput
	CostCentreAllocations []CostCentreAllocInput
}

// BillAllocationInput is a bill reference line for receivable/payable tracking.
type BillAllocationInput struct {
	Name     string
	BillType string  // "New Ref" | "Agst Ref" | "Advance"
	Amount   float64
	DueDate  time.Time
}

// BankAllocationInput carries cheque/NEFT/RTGS details.
type BankAllocationInput struct {
	InstNumber            string
	UniqueReferenceNumber string
	TransactionType       string
}

// InventoryEntryInput is one stock line in an invoice voucher.
type InventoryEntryInput struct {
	StockItemName string
	GodownName    string
	Rate          float64
	RateUOM       string
	Amount        float64
	BilledQty     float64
	BilledQtyUOM  string
	HSNCode       string
	AccountLedger string // income / purchase account
}

// VoucherInput is the full data needed to create/alter a voucher in Tally.
type VoucherInput struct {
	CompanyName     string    // SVCURRENTCOMPANY — set so Tally writes to the right company
	Action          WriteAction
	GUID            string    // required for Alter/Cancel; new UUID for Create
	VoucherType     string    // Sales | Purchase | Payment | Receipt | Journal | Contra
	ObjView         string    // "Invoice Voucher View" | "Accounting Voucher View"
	Date            time.Time
	VoucherNumber   string
	ReferenceNumber string
	ReferenceDate   time.Time
	PartyLedgerName string
	Narration       string
	IsInvoice       bool
	PlaceOfSupply   string
	IsRCMApplicable bool
	EWayBillNumber  string
	IRNNO           string
	LedgerEntries   []LedgerEntryInput
	InventoryEntries []InventoryEntryInput
}

// ---- Build voucher XML ------------------------------------------------------

// BuildVoucher returns the full import XML envelope for a voucher write.
// CRITICAL: Validate that all AMOUNT values sum to zero before calling this.
// Tally returns ERRORS="Voucher totals do not match" for imbalanced vouchers.
func BuildVoucher(v VoucherInput) (string, error) {
	if err := validateVoucherBalance(v); err != nil {
		return "", err
	}
	body := buildVoucherXML(v)
	return tally.BuildImport(tally.ImportRequest{
		Type:    "Vouchers",
		Company: v.CompanyName,
		Body:    body,
	}), nil
}

// validateVoucherBalance checks that all ledger amounts sum to zero.
// PRD edge case #18: never send imbalanced voucher to Tally.
func validateVoucherBalance(v VoucherInput) error {
	var sum float64
	for _, le := range v.LedgerEntries {
		sum += le.Amount
	}
	// Allow for floating-point epsilon of 0.001 (paise)
	if sum > 0.001 || sum < -0.001 {
		return fmt.Errorf("writer: voucher %q is imbalanced: ledger sum = %.4f (must be 0)", v.GUID, sum)
	}
	return nil
}

func buildVoucherXML(v VoucherInput) string {
	var b strings.Builder

	// Always use Accounting Voucher View for imports — Invoice Voucher View
	// requires INVENTORYENTRIESIN.LIST with specific tag semantics that differ
	// from Tally's export format. Accounting View with ledger entries only is
	// the reliable way to import vouchers.
	objView := "Accounting Voucher View"

	fmt.Fprintf(&b, `<VOUCHER ACTION="%s" OBJVIEW="%s"`, v.Action, objView)
	if v.Action == ActionAlter || v.Action == ActionCancel {
		fmt.Fprintf(&b, ` TAGNAME="GUID" TAGVALUE="%s"`, xmlEsc(v.GUID))
	}
	b.WriteString(">")

	fmt.Fprintf(&b, "\n  <GUID>%s</GUID>", xmlEsc(v.GUID))
	fmt.Fprintf(&b, "\n  <DATE>%s</DATE>", v.Date.UTC().Format("20060102"))
	fmt.Fprintf(&b, "\n  <VOUCHERTYPENAME>%s</VOUCHERTYPENAME>", xmlEsc(v.VoucherType))
	if v.VoucherNumber != "" {
		fmt.Fprintf(&b, "\n  <VOUCHERNUMBER>%s</VOUCHERNUMBER>", xmlEsc(v.VoucherNumber))
	}
	if v.ReferenceNumber != "" {
		fmt.Fprintf(&b, "\n  <REFERENCENUMBER>%s</REFERENCENUMBER>", xmlEsc(v.ReferenceNumber))
	}
	if !v.ReferenceDate.IsZero() {
		fmt.Fprintf(&b, "\n  <REFERENCEDATE>%s</REFERENCEDATE>", v.ReferenceDate.UTC().Format("20060102"))
	}
	if v.PartyLedgerName != "" {
		fmt.Fprintf(&b, "\n  <PARTYLEDGERNAME>%s</PARTYLEDGERNAME>", xmlEsc(v.PartyLedgerName))
	}
	if v.Narration != "" {
		fmt.Fprintf(&b, "\n  <NARRATION>%s</NARRATION>", xmlEsc(v.Narration))
	}
	if v.PlaceOfSupply != "" {
		fmt.Fprintf(&b, "\n  <PLACEOFSUPPLY>%s</PLACEOFSUPPLY>", xmlEsc(v.PlaceOfSupply))
	}
	if v.IsRCMApplicable {
		b.WriteString("\n  <ISRCMAPPLICABLE>Yes</ISRCMAPPLICABLE>")
	}
	if v.EWayBillNumber != "" {
		fmt.Fprintf(&b, "\n  <EWAYBILLNUMBER>%s</EWAYBILLNUMBER>", xmlEsc(v.EWayBillNumber))
	}
	if v.IRNNO != "" {
		fmt.Fprintf(&b, "\n  <IRNNO>%s</IRNNO>", xmlEsc(v.IRNNO))
	}

	// Ledger entries
	for _, le := range v.LedgerEntries {
		b.WriteString("\n  <ALLLEDGERENTRIES.LIST>")
		fmt.Fprintf(&b, "\n    <LEDGERNAME>%s</LEDGERNAME>", xmlEsc(le.LedgerName))
		b.WriteString("\n    <ISDEEMEDPOSITIVE>")
		if le.IsDeemedPositive {
			b.WriteString("Yes")
		} else {
			b.WriteString("No")
		}
		b.WriteString("</ISDEEMEDPOSITIVE>")
		if le.IsPartyLedger {
			b.WriteString("\n    <ISPARTYLEDGER>Yes</ISPARTYLEDGER>")
		}
		fmt.Fprintf(&b, "\n    <AMOUNT>%.2f</AMOUNT>", le.Amount)

		for _, ba := range le.BillAllocations {
			b.WriteString("\n    <BILLALLOCATIONS.LIST>")
			fmt.Fprintf(&b, "\n      <NAME>%s</NAME>", xmlEsc(ba.Name))
			fmt.Fprintf(&b, "\n      <BILLTYPE>%s</BILLTYPE>", xmlEsc(ba.BillType))
			fmt.Fprintf(&b, "\n      <AMOUNT>%.2f</AMOUNT>", ba.Amount)
			if !ba.DueDate.IsZero() {
				fmt.Fprintf(&b, "\n      <DUEDATE>%s</DUEDATE>", ba.DueDate.UTC().Format("20060102"))
			}
			b.WriteString("\n    </BILLALLOCATIONS.LIST>")
		}

		for _, bk := range le.BankAllocations {
			b.WriteString("\n    <BANKALLOCATIONS.LIST>")
			if bk.InstNumber != "" {
				fmt.Fprintf(&b, "\n      <INSTNUMBER>%s</INSTNUMBER>", xmlEsc(bk.InstNumber))
			}
			if bk.UniqueReferenceNumber != "" {
				fmt.Fprintf(&b, "\n      <UNIQUEREFERENCENUMBER>%s</UNIQUEREFERENCENUMBER>",
					xmlEsc(bk.UniqueReferenceNumber))
			}
			if bk.TransactionType != "" {
				fmt.Fprintf(&b, "\n      <TRANSACTIONTYPE>%s</TRANSACTIONTYPE>", xmlEsc(bk.TransactionType))
			}
			b.WriteString("\n    </BANKALLOCATIONS.LIST>")
		}

		for _, cc := range le.CostCentreAllocations {
			b.WriteString("\n    <COSTCENTREALLOCATIONS.LIST>")
			fmt.Fprintf(&b, "\n      <NAME>%s</NAME>", xmlEsc(cc.CostCentreName))
			fmt.Fprintf(&b, "\n      <AMOUNT>%.2f</AMOUNT>", cc.Amount)
			b.WriteString("\n    </COSTCENTREALLOCATIONS.LIST>")
		}

		b.WriteString("\n  </ALLLEDGERENTRIES.LIST>")
	}

	// NOTE: Inventory entries (INVENTORYENTRIESIN.LIST) are intentionally
	// omitted for imports — ledger entries alone are sufficient for correct
	// accounting. Invoice Voucher View inventory requires special tag names
	// (INVENTORYENTRIESIN vs INVENTORYENTRIESOUT) and auto-generates ledger
	// entries, which conflicts with explicitly provided ledger entries.

	b.WriteString("\n</VOUCHER>")
	return b.String()
}

// xmlEsc is package-local to avoid import cycles from tally package.
func xmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

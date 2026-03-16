package reader

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/parser"
)

// ---- Voucher type hierarchy -------------------------------------------------

// Voucher holds a fully parsed Tally voucher from API 10 (delta) or API 11 (single).
type Voucher struct {
	GUID              string          `json:"guid"`
	Date              time.Time       `json:"date"`
	AlteredOn         time.Time       `json:"alteredOn"`
	VoucherTypeName   string          `json:"voucherTypeName"`
	VoucherNumber     string          `json:"voucherNumber"`
	ReferenceNumber   string          `json:"referenceNumber"`
	ReferenceDate     time.Time       `json:"referenceDate"`
	PartyLedgerName   string          `json:"partyLedgerName"`
	IsInvoice         bool            `json:"isInvoice"`
	IsOptional        bool            `json:"isOptional"`
	IsCancelled       bool            `json:"isCancelled"`
	IsDeleted         bool            `json:"isDeleted"`
	Narration         string          `json:"narration"`
	ForeignCurrency   string          `json:"foreignCurrency"`
	ForeignAmount     float64         `json:"foreignAmount"`
	PlaceOfSupply     string          `json:"placeOfSupply"`
	PartyGSTIN        string          `json:"partyGSTIN"`
	IsRCMApplicable   bool            `json:"isRCMApplicable"`
	EWayBillNumber    string          `json:"eWayBillNumber"`
	IRNNO             string          `json:"irnNo"`
	LedgerEntries     []LedgerEntry   `json:"ledgerEntries"`
	InventoryEntries  []InventoryEntry `json:"inventoryEntries"`
}

// LedgerEntry is one line in ALLLEDGERENTRIES.
type LedgerEntry struct {
	LedgerName       string           `json:"ledgerName"`
	IsDeemedPositive bool             `json:"isDeemedPositive"`
	IsPartyLedger    bool             `json:"isPartyLedger"`
	Amount           float64          `json:"amount"`
	BillAllocations  []BillAllocation `json:"billAllocations"`
	BankAllocations  []BankAllocation `json:"bankAllocations"`
}

// BillAllocation is one entry in BILLALLOCATIONS.LIST.
type BillAllocation struct {
	Name     string    `json:"name"`
	BillType string    `json:"billType"`
	Amount   float64   `json:"amount"`
	DueDate  time.Time `json:"dueDate"`
}

// BankAllocation is one entry in BANKALLOCATIONS.LIST.
type BankAllocation struct {
	InstNumber            string `json:"instNumber"`
	UniqueReferenceNumber string `json:"uniqueReferenceNumber"`
	TransactionType       string `json:"transactionType"`
}

// InventoryEntry is one line in ALLINVENTORYENTRIES.
type InventoryEntry struct {
	StockItemName     string    `json:"stockItemName"`
	Rate              float64   `json:"rate"`
	RateUOM           string    `json:"rateUOM"`
	Amount            float64   `json:"amount"`
	ActualQty         float64   `json:"actualQty"`
	ActualQtyUOM      string    `json:"actualQtyUOM"`
	BilledQty         float64   `json:"billedQty"`
	BilledQtyUOM      string    `json:"billedQtyUOM"`
	HSNCode           string    `json:"hsnCode"`
	GSTOvrdnRate      float64   `json:"gstOvrdnRate"`
	GodownName        string    `json:"godownName"`
	ExpiryDate        time.Time `json:"expiryDate"`
	AccountLedgerName string    `json:"accountLedgerName"`
}

// ---- XML decode targets (API 10/11) ----------------------------------------

type vouchersEnvelope struct {
	XMLName xml.Name     `xml:"ENVELOPE"`
	Body    vouchersBody `xml:"BODY"`
}
type vouchersBody struct{ Data vouchersData `xml:"DATA"` }
type vouchersData struct{ Collection vouchersCollection `xml:"COLLECTION"` }
type vouchersCollection struct{ Vouchers []xmlVoucher `xml:"VOUCHER"` }

type xmlVoucher struct {
	GUID              string              `xml:"GUID"`
	Date              string              `xml:"DATE"`
	AlteredOn         string              `xml:"ALTEREDON"`
	VoucherTypeName   string              `xml:"VOUCHERTYPENAME"`
	VoucherNumber     string              `xml:"VOUCHERNUMBER"`
	ReferenceNumber   string              `xml:"REFERENCE"` // Tally returns reference under <REFERENCE>, not <REFERENCENUMBER>
	ReferenceDate     string              `xml:"REFERENCEDATE"`
	PartyLedgerName   string              `xml:"PARTYLEDGERNAME"`
	IsInvoice         string              `xml:"ISINVOICE"`
	IsOptional        string              `xml:"ISOPTIONAL"`
	IsCancelled       string              `xml:"ISCANCELLED"`
	IsDeleted         string              `xml:"ISDELETED"`
	Narration         string              `xml:"NARRATION"`
	ForeignCurrency   string              `xml:"FOREIGNCURRENCY"`
	ForeignAmount     string              `xml:"FOREIGNAMOUNT"`
	PlaceOfSupply     string              `xml:"PLACEOFSUPPLY"`
	PartyGSTIN        string              `xml:"PARTYGSTIN"`
	IsRCMApplicable   string              `xml:"ISRCMAPPLICABLE"`
	EWayBillNumber    string              `xml:"EWAYBILLNUMBER"`
	IRNNO             string              `xml:"IRNNO"`
	LedgerEntries     []xmlLedgerEntry    `xml:"ALLLEDGERENTRIES.LIST"`
	InventoryEntries  []xmlInventoryEntry `xml:"ALLINVENTORYENTRIES.LIST"`
}

type xmlLedgerEntry struct {
	LedgerName       string            `xml:"LEDGERNAME"`
	IsDeemedPositive string            `xml:"ISDEEMEDPOSITIVE"`
	IsPartyLedger    string            `xml:"ISPARTYLEDGER"`
	Amount           string            `xml:"AMOUNT"`
	BillAllocations  []xmlBillAlloc    `xml:"BILLALLOCATIONS.LIST"`
	BankAllocations  []xmlBankAlloc    `xml:"BANKALLOCATIONS.LIST"`
}

type xmlBillAlloc struct {
	Name     string `xml:"NAME"`
	BillType string `xml:"BILLTYPE"`
	Amount   string `xml:"AMOUNT"`
	DueDate  string `xml:"DUEDATE"`
}

type xmlBankAlloc struct {
	InstNumber            string `xml:"INSTNUMBER"`
	UniqueReferenceNumber string `xml:"UNIQUEREFERENCENUMBER"`
	TransactionType       string `xml:"TRANSACTIONTYPE"`
}

type xmlInventoryEntry struct {
	StockItemName    string            `xml:"STOCKITEMNAME"`
	Rate             string            `xml:"RATE"`
	Amount           string            `xml:"AMOUNT"`
	ActualQty        string            `xml:"ACTUALQTY"`
	BilledQty        string            `xml:"BILLEDQTY"`
	HSNCode          string            `xml:"GSTHSNNAME"` // Tally returns HSN under GSTHSNNAME, not HSNCODE
	GSTOvrdnRate     string            `xml:"GSTOVRDNRATE"`
	BatchAllocations []xmlBatchAlloc   `xml:"BATCHALLOCATIONS.LIST"`
	AcctAllocations  []xmlAcctAlloc    `xml:"ACCOUNTINGALLOCATIONS.LIST"`
}

type xmlBatchAlloc struct {
	GodownName string `xml:"GODOWNNAME"`
	ExpiryDate string `xml:"EXPIRYDATE"`
}

type xmlAcctAlloc struct {
	LedgerName string `xml:"LEDGERNAME"`
}

// ---- Fetch fields -----------------------------------------------------------

var voucherFetchFields = []string{
	"GUID", "Date", "VoucherTypeName", "VoucherNumber",
	"Reference", "ReferenceDate", "PartyLedgerName",
	"IsInvoice", "IsOptional", "IsCancelled", "IsDeleted", "Narration",
	"ForeignCurrency", "ForeignAmount", "PlaceOfSupply",
	"PartyGSTIN", "IsRCMApplicable", "EWayBillNumber", "IRNNO",
	"ALTEREDON", "AllLedgerEntries", "AllInventoryEntries", "BillAllocations",
}

// ---- Public API -------------------------------------------------------------

// ListDeltaVouchers fetches all vouchers altered since the given timestamp (API 10).
// sinceTimestamp format: "yyyymmddHHmmss" — store from parser.FormatTallyDate().
// fromDate/toDate scope the query to a financial year; zero values use a wide default.
// IsOptional=Yes vouchers are returned as-is; callers MUST skip them (edge case #1).
//
// For large companies this splits the date range into monthly chunks to avoid
// overwhelming Tally with a single massive XML response (which causes Tally to
// freeze and the HTTP request to time out).
func ListDeltaVouchers(ctx context.Context, c *tally.Client, company, sinceTimestamp string, fromDate, toDate time.Time) ([]Voucher, error) {
	filter := ""
	if sinceTimestamp != "" {
		filter = "$$IsAlteredSince:" + sinceTimestamp
	}
	// Default to a wide date range so Tally does not restrict to the current FY.
	if fromDate.IsZero() {
		fromDate = time.Date(2000, 4, 1, 0, 0, 0, 0, time.UTC)
	}
	if toDate.IsZero() {
		toDate = time.Now().UTC().AddDate(1, 0, 0)
	}

	// Split into monthly chunks to keep each request small.
	chunks := monthlyChunks(fromDate, toDate)
	var all []Voucher
	seen := make(map[string]bool) // deduplicate by GUID across chunk boundaries

	for i, chunk := range chunks {
		fmt.Printf("  Fetching vouchers chunk %d/%d (%s – %s)...\n",
			i+1, len(chunks),
			chunk.from.Format("2006-01-02"), chunk.to.Format("2006-01-02"))

		xmlBody := tally.BuildCollection(tally.CollectionRequest{
			Name:   "TBDeltaVouchers",
			Type:   "Voucher",
			Fetch:  voucherFetchFields,
			Filter: filter,
			Vars:   tally.StaticVars{Company: company, FromDate: chunk.from, ToDate: chunk.to},
		})
		raw, err := c.Post(ctx, xmlBody)
		if err != nil {
			return nil, fmt.Errorf("reader.ListDeltaVouchers (chunk %d/%d): %w", i+1, len(chunks), err)
		}
		vouchers, err := parseVouchersResponse(raw)
		if err != nil {
			return nil, fmt.Errorf("reader.ListDeltaVouchers (chunk %d/%d): %w", i+1, len(chunks), err)
		}
		for _, v := range vouchers {
			if !seen[v.GUID] {
				seen[v.GUID] = true
				all = append(all, v)
			}
		}
		fmt.Printf("  Chunk %d/%d: got %d vouchers (total so far: %d)\n",
			i+1, len(chunks), len(vouchers), len(all))
	}

	return all, nil
}

// dateRange represents a from/to pair for chunked fetching.
type dateRange struct {
	from, to time.Time
}

// monthlyChunks splits a date range into calendar-month chunks.
func monthlyChunks(from, to time.Time) []dateRange {
	var chunks []dateRange
	cursor := from
	for cursor.Before(to) {
		// End of this month (or toDate, whichever is earlier)
		nextMonth := time.Date(cursor.Year(), cursor.Month()+1, 1, 0, 0, 0, 0, cursor.Location())
		end := nextMonth.AddDate(0, 0, -1) // last day of cursor's month
		if end.After(to) {
			end = to
		}
		chunks = append(chunks, dateRange{from: cursor, to: end})
		cursor = nextMonth
	}
	if len(chunks) == 0 {
		chunks = append(chunks, dateRange{from: from, to: to})
	}
	return chunks
}

// GetVoucherByGUID fetches a single voucher by GUID (API 11) — full detail.
func GetVoucherByGUID(ctx context.Context, c *tally.Client, company, guid string) (*Voucher, error) {
	xmlBody := tally.BuildObject(tally.ObjectRequest{
		SubType: "Voucher",
		IDType:  "GUID",
		IDValue: guid,
		Vars:    tally.StaticVars{Company: company},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.GetVoucherByGUID: %w", err)
	}
	// Single object response wraps the voucher directly under ENVELOPE/BODY/DATA.
	// Tally returns it inside a single-element collection in practice.
	vouchers, err := parseVouchersResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("reader.GetVoucherByGUID: %w", err)
	}
	if len(vouchers) == 0 {
		return nil, fmt.Errorf("reader.GetVoucherByGUID: GUID %q not found", guid)
	}
	return &vouchers[0], nil
}

// ---- Internal parsing -------------------------------------------------------

func parseVouchersResponse(raw []byte) ([]Voucher, error) {
	var env vouchersEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parseVouchersResponse: xml: %w", err)
	}
	out := make([]Voucher, 0, len(env.Body.Data.Collection.Vouchers))
	for _, xv := range env.Body.Data.Collection.Vouchers {
		v := mapXMLVoucher(xv)
		out = append(out, v)
	}
	return out, nil
}

func mapXMLVoucher(xv xmlVoucher) Voucher {
	v := Voucher{
		GUID:            xv.GUID,
		VoucherTypeName: xv.VoucherTypeName,
		VoucherNumber:   xv.VoucherNumber,
		ReferenceNumber: xv.ReferenceNumber,
		PartyLedgerName: xv.PartyLedgerName,
		IsInvoice:       parser.ParseBool(xv.IsInvoice),
		IsOptional:      parser.ParseBool(xv.IsOptional),
		IsCancelled:     parser.ParseBool(xv.IsCancelled),
		IsDeleted:       parser.ParseBool(xv.IsDeleted),
		Narration:       xv.Narration,
		ForeignCurrency: xv.ForeignCurrency,
		ForeignAmount:   parser.ParseAmount(xv.ForeignAmount),
		PlaceOfSupply:   xv.PlaceOfSupply,
		PartyGSTIN:      xv.PartyGSTIN,
		IsRCMApplicable: parser.ParseBool(xv.IsRCMApplicable),
		EWayBillNumber:  xv.EWayBillNumber,
		IRNNO:           xv.IRNNO,
	}
	if t, err := parser.ParseDate(xv.Date); err == nil {
		v.Date = t
	}
	if t, err := parser.ParseDate(xv.AlteredOn); err == nil {
		v.AlteredOn = t
	}
	if t, err := parser.ParseDate(xv.ReferenceDate); err == nil {
		v.ReferenceDate = t
	}

	// Ledger entries
	v.LedgerEntries = make([]LedgerEntry, 0, len(xv.LedgerEntries))
	for _, xl := range xv.LedgerEntries {
		le := LedgerEntry{
			LedgerName:       xl.LedgerName,
			IsDeemedPositive: parser.ParseBool(xl.IsDeemedPositive),
			IsPartyLedger:    parser.ParseBool(xl.IsPartyLedger),
			Amount:           parser.ParseAmount(xl.Amount),
		}
		for _, xb := range xl.BillAllocations {
			ba := BillAllocation{
				Name:     xb.Name,
				BillType: xb.BillType,
				Amount:   parser.ParseAmount(xb.Amount),
			}
			if t, err := parser.ParseDate(xb.DueDate); err == nil {
				ba.DueDate = t
			}
			le.BillAllocations = append(le.BillAllocations, ba)
		}
		for _, xbk := range xl.BankAllocations {
			le.BankAllocations = append(le.BankAllocations, BankAllocation{ //nolint:gosimple // different struct tags (xml vs json)
				InstNumber:            xbk.InstNumber,
				UniqueReferenceNumber: xbk.UniqueReferenceNumber,
				TransactionType:       xbk.TransactionType,
			})
		}
		v.LedgerEntries = append(v.LedgerEntries, le)
	}

	// Inventory entries
	v.InventoryEntries = make([]InventoryEntry, 0, len(xv.InventoryEntries))
	for _, xi := range xv.InventoryEntries {
		ie := InventoryEntry{
			StockItemName: xi.StockItemName,
			Amount:        parser.ParseAmount(xi.Amount),
			HSNCode:       xi.HSNCode,
			GSTOvrdnRate:  parser.ParseAmount(xi.GSTOvrdnRate),
		}
		ie.Rate, ie.RateUOM, _ = parser.ParseRate(xi.Rate)
		ie.ActualQty, ie.ActualQtyUOM, _ = parser.ParseQuantity(xi.ActualQty)
		ie.BilledQty, ie.BilledQtyUOM, _ = parser.ParseQuantity(xi.BilledQty)

		if len(xi.BatchAllocations) > 0 {
			ie.GodownName = xi.BatchAllocations[0].GodownName
			if t, err := parser.ParseDate(xi.BatchAllocations[0].ExpiryDate); err == nil {
				ie.ExpiryDate = t
			}
		}
		if len(xi.AcctAllocations) > 0 {
			ie.AccountLedgerName = xi.AcctAllocations[0].LedgerName
		}
		v.InventoryEntries = append(v.InventoryEntries, ie)
	}

	return v
}

// CountVouchers returns the number of vouchers in the given company and date range.
// It fetches only GUID (minimal payload) to avoid large response sizes.
func CountVouchers(ctx context.Context, c *tally.Client, company string, fromDate, toDate time.Time) (int, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBVoucherCount",
		Type:  "Voucher",
		Fetch: []string{"GUID"},
		Vars:  tally.StaticVars{Company: company, FromDate: fromDate, ToDate: toDate},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return 0, fmt.Errorf("reader.CountVouchers: %w", err)
	}
	var env vouchersEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return 0, fmt.Errorf("reader.CountVouchers: xml: %w", err)
	}
	return len(env.Body.Data.Collection.Vouchers), nil
}

package reader

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/parser"
)

// ---- Stock Voucher type hierarchy -------------------------------------------

// StockVoucher holds a fully parsed Tally stock movement voucher.
// Covers Stock Journal, Physical Stock, Delivery Note, and Receipt Note.
type StockVoucher struct {
	GUID             string                    `json:"guid"`
	Date             time.Time                 `json:"date"`
	VoucherTypeName  string                    `json:"voucherTypeName"`
	VoucherNumber    string                    `json:"voucherNumber"`
	Narration        string                    `json:"narration"`
	IsDeleted        bool                      `json:"isDeleted"`
	IsCancelled      bool                      `json:"isCancelled"`
	IsOptional       bool                      `json:"isOptional"`
	LedgerEntries    []StockVoucherLedgerEntry `json:"ledgerEntries,omitempty"`
	InventoryEntries []StockInventoryEntry     `json:"inventoryEntries,omitempty"`
}

// StockVoucherLedgerEntry is one ledger line in a stock voucher (ALLLEDGERENTRIES.LIST).
type StockVoucherLedgerEntry struct {
	LedgerName string  `json:"ledgerName"`
	Amount     float64 `json:"amount"`
}

// StockInventoryEntry is one inventory line in a stock voucher (ALLINVENTORYENTRIES.LIST).
type StockInventoryEntry struct {
	StockItemName    string  `json:"stockItemName"`
	ActualQty        float64 `json:"actualQty"`
	ActualQtyUOM     string  `json:"actualQtyUOM"`
	BilledQty        float64 `json:"billedQty"`
	BilledQtyUOM     string  `json:"billedQtyUOM"`
	Rate             float64 `json:"rate"`
	Amount           float64 `json:"amount"`
	GodownName       string  `json:"godownName,omitempty"`
	BatchName        string  `json:"batchName,omitempty"`
	SourceGodownName string  `json:"sourceGodownName,omitempty"`
}

// ---- XML decode targets -----------------------------------------------------

type stockVouchersEnvelope struct {
	XMLName xml.Name           `xml:"ENVELOPE"`
	Body    stockVouchersBody  `xml:"BODY"`
}
type stockVouchersBody struct{ Data stockVouchersData `xml:"DATA"` }
type stockVouchersData struct{ Collection stockVouchersCollection `xml:"COLLECTION"` }
type stockVouchersCollection struct{ Vouchers []xmlStockVoucher `xml:"VOUCHER"` }

type xmlStockVoucher struct {
	GUID             string                    `xml:"GUID"`
	Date             string                    `xml:"DATE"`
	VoucherTypeName  string                    `xml:"VOUCHERTYPENAME"`
	VoucherNumber    string                    `xml:"VOUCHERNUMBER"`
	Narration        string                    `xml:"NARRATION"`
	IsDeleted        string                    `xml:"ISDELETED"`
	IsCancelled      string                    `xml:"ISCANCELLED"`
	IsOptional       string                    `xml:"ISOPTIONAL"`
	LedgerEntries    []xmlStockVoucherLedger   `xml:"ALLLEDGERENTRIES.LIST"`
	InventoryEntries []xmlStockInventoryEntry  `xml:"ALLINVENTORYENTRIES.LIST"`
}

type xmlStockVoucherLedger struct {
	LedgerName string `xml:"LEDGERNAME"`
	Amount     string `xml:"AMOUNT"`
}

type xmlStockInventoryEntry struct {
	StockItemName    string               `xml:"STOCKITEMNAME"`
	ActualQty        string               `xml:"ACTUALQTY"`
	BilledQty        string               `xml:"BILLEDQTY"`
	Rate             string               `xml:"RATE"`
	Amount           string               `xml:"AMOUNT"`
	BatchAllocations []xmlStockBatchAlloc `xml:"BATCHALLOCATIONS.LIST"`
}

type xmlStockBatchAlloc struct {
	GodownName       string `xml:"GODOWNNAME"`
	BatchName        string `xml:"BATCHNAME"`
	SourceGodownName string `xml:"SOURCEGODOWNNAME"`
}

// ---- Fetch fields -----------------------------------------------------------

var stockVoucherFetchFields = []string{
	"GUID", "Date", "VoucherTypeName", "VoucherNumber", "Narration",
	"IsDeleted", "IsCancelled", "IsOptional",
	"AllLedgerEntries", "AllInventoryEntries",
}

// stockVoucherTypeFilter is used as a combined OR filter expression.
// Tally evaluates each collection filter individually so we use a single
// multi-condition formula joined with OR.
const stockVoucherTypeFilter = `$VoucherTypeName = "Stock Journal" OR $VoucherTypeName = "Physical Stock" OR $VoucherTypeName = "Delivery Note" OR $VoucherTypeName = "Receipt Note"`

// ---- Public API -------------------------------------------------------------

// FetchStockVouchers fetches all stock movement vouchers (Stock Journal, Physical Stock,
// Delivery Note, Receipt Note) in the given date range from Tally.
//
// sinceTimestamp — optional yyyymmddHHmmss watermark; pass "" for a full fetch.
// Uses monthly chunking (same strategy as ListDeltaVouchers) to avoid overwhelming Tally.
func FetchStockVouchers(ctx context.Context, c *tally.Client, company, sinceTimestamp string, fromDate, toDate time.Time) ([]StockVoucher, error) {
	// Build combined filter: type-filter AND optional altered-since filter.
	filter := stockVoucherTypeFilter
	if sinceTimestamp != "" {
		filter = "(" + stockVoucherTypeFilter + ") AND $$IsAlteredSince:" + sinceTimestamp
	}

	// Default to wide date range when zero values supplied.
	if fromDate.IsZero() {
		fromDate = time.Date(2000, 4, 1, 0, 0, 0, 0, time.UTC)
	}
	if toDate.IsZero() {
		toDate = time.Now().UTC().AddDate(1, 0, 0)
	}

	// Split into monthly chunks to keep each request small.
	chunks := monthlyChunks(fromDate, toDate)
	var all []StockVoucher
	seen := make(map[string]bool) // deduplicate by GUID across chunk boundaries

	for i, chunk := range chunks {
		fmt.Printf("  Fetching stock vouchers chunk %d/%d (%s – %s)...\n",
			i+1, len(chunks),
			chunk.from.Format("2006-01-02"), chunk.to.Format("2006-01-02"))

		xmlBody := tally.BuildCollection(tally.CollectionRequest{
			Name:   "TBStockVouchers",
			Type:   "Voucher",
			Fetch:  stockVoucherFetchFields,
			Filter: filter,
			Vars:   tally.StaticVars{Company: company, FromDate: chunk.from, ToDate: chunk.to},
		})
		raw, err := c.Post(ctx, xmlBody)
		if err != nil {
			return nil, fmt.Errorf("reader.FetchStockVouchers (chunk %d/%d): %w", i+1, len(chunks), err)
		}
		vouchers, err := parseStockVouchersResponse(raw)
		if err != nil {
			return nil, fmt.Errorf("reader.FetchStockVouchers (chunk %d/%d): %w", i+1, len(chunks), err)
		}
		for _, v := range vouchers {
			if !seen[v.GUID] {
				seen[v.GUID] = true
				all = append(all, v)
			}
		}
		fmt.Printf("  Stock voucher chunk %d/%d: got %d (total so far: %d)\n",
			i+1, len(chunks), len(vouchers), len(all))
	}

	return all, nil
}

// ---- Internal parsing -------------------------------------------------------

func parseStockVouchersResponse(raw []byte) ([]StockVoucher, error) {
	var env stockVouchersEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("parseStockVouchersResponse: xml: %w", err)
	}
	out := make([]StockVoucher, 0, len(env.Body.Data.Collection.Vouchers))
	for _, xv := range env.Body.Data.Collection.Vouchers {
		out = append(out, mapXMLStockVoucher(xv))
	}
	return out, nil
}

func mapXMLStockVoucher(xv xmlStockVoucher) StockVoucher {
	v := StockVoucher{
		GUID:            xv.GUID,
		VoucherTypeName: xv.VoucherTypeName,
		VoucherNumber:   xv.VoucherNumber,
		Narration:       xv.Narration,
		IsDeleted:       parser.ParseBool(xv.IsDeleted),
		IsCancelled:     parser.ParseBool(xv.IsCancelled),
		IsOptional:      parser.ParseBool(xv.IsOptional),
	}
	if t, err := parser.ParseDate(xv.Date); err == nil {
		v.Date = t
	}

	// Ledger entries
	v.LedgerEntries = make([]StockVoucherLedgerEntry, 0, len(xv.LedgerEntries))
	for _, xl := range xv.LedgerEntries {
		v.LedgerEntries = append(v.LedgerEntries, StockVoucherLedgerEntry{
			LedgerName: xl.LedgerName,
			Amount:     parser.ParseAmount(xl.Amount),
		})
	}

	// Inventory entries
	v.InventoryEntries = make([]StockInventoryEntry, 0, len(xv.InventoryEntries))
	for _, xi := range xv.InventoryEntries {
		ie := StockInventoryEntry{
			Amount: parser.ParseAmount(xi.Amount),
		}
		ie.StockItemName = xi.StockItemName
		ie.Rate, _, _ = parser.ParseRate(xi.Rate)
		ie.ActualQty, ie.ActualQtyUOM, _ = parser.ParseQuantity(xi.ActualQty)
		ie.BilledQty, ie.BilledQtyUOM, _ = parser.ParseQuantity(xi.BilledQty)
		if len(xi.BatchAllocations) > 0 {
			ie.GodownName = xi.BatchAllocations[0].GodownName
			ie.BatchName = xi.BatchAllocations[0].BatchName
			ie.SourceGodownName = xi.BatchAllocations[0].SourceGodownName
		}
		v.InventoryEntries = append(v.InventoryEntries, ie)
	}

	return v
}

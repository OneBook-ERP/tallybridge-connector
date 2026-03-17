package reader

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/tallybridge/connector/tally"
)

// GetVoucherCounts queries Tally for the count of vouchers per type between
// fromDate and toDate (both in "2006-01-02" format; empty = no date bound).
// Returns a map like {"Sales": 450, "Purchase": 120, "Payment": 80, ...}.
func GetVoucherCounts(tallyURL, companyName, fromDate, toDate string) (map[string]int, error) {
	c := tally.NewClient(tally.WithAddr(tallyURL))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var from, to time.Time
	if fromDate != "" {
		if t, err := time.Parse("2006-01-02", fromDate); err == nil {
			from = t
		}
	}
	if toDate != "" {
		if t, err := time.Parse("2006-01-02", toDate); err == nil {
			to = t
		}
	}

	voucherTypes := []string{"Sales", "Purchase", "Payment", "Receipt", "Journal", "Stock Journal", "Delivery Note", "Purchase Order"}
	result := make(map[string]int, len(voucherTypes))

	for _, vtype := range voucherTypes {
		// TDL formulae field reference uses $FieldName syntax.
		safeName := strings.ReplaceAll(vtype, " ", "")
		xmlBody := tally.BuildCollection(tally.CollectionRequest{
			Name:   "TBCount" + safeName,
			Type:   "Voucher",
			Fetch:  []string{"GUID"},
			Filter: "$VoucherTypeName = \"" + vtype + "\"",
			Vars:   tally.StaticVars{Company: companyName, FromDate: from, ToDate: to},
		})
		raw, err := c.Post(ctx, xmlBody)
		if err != nil {
			// Skip individual type errors — not fatal
			continue
		}
		var env vouchersEnvelope
		if err := xml.Unmarshal(raw, &env); err != nil {
			continue
		}
		n := len(env.Body.Data.Collection.Vouchers)
		if n > 0 {
			result[vtype] = n
		}
	}

	return result, nil
}

// GetMasterCounts queries Tally for ledger counts grouped by key parent groups
// and the total stock item count.
// Returns a map like {"Sundry Debtors": 45, "Sundry Creditors": 20, "stockItems": 80}.
func GetMasterCounts(tallyURL, companyName string) (map[string]int, error) {
	c := tally.NewClient(tally.WithAddr(tallyURL))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result := make(map[string]int)

	// Query ledger count by parent group for key groups.
	parentGroups := []string{"Sundry Debtors", "Sundry Creditors", "Bank Accounts", "Cash-in-Hand"}
	for _, group := range parentGroups {
		safeName := strings.ReplaceAll(group, " ", "")
		safeName = strings.ReplaceAll(safeName, "-", "")
		xmlBody := tally.BuildCollection(tally.CollectionRequest{
			Name:   fmt.Sprintf("TBLedgerCount%s", safeName),
			Type:   "Ledger",
			Fetch:  []string{"Name"},
			Filter: "$Parent = \"" + group + "\"",
			Vars:   tally.StaticVars{Company: companyName},
		})
		raw, err := c.Post(ctx, xmlBody)
		if err != nil {
			continue
		}
		var env ledgersEnvelope
		if err := xml.Unmarshal(raw, &env); err != nil {
			continue
		}
		n := len(env.Body.Data.Collection.Ledgers)
		if n > 0 {
			result[group] = n
		}
	}

	// Query total stock item count.
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBStockItemCount",
		Type:  "Stock Item",
		Fetch: []string{"Name"},
		Vars:  tally.StaticVars{Company: companyName},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return result, nil
	}
	var env stockItemsEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return result, nil
	}
	n := len(env.Body.Data.Collection.Items)
	if n > 0 {
		result["stockItems"] = n
	}

	return result, nil
}

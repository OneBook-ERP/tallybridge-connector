package sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tallybridge/connector/store"
	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/reader"
)

// masterHash computes a stable SHA-256 hex digest of any JSON-serialisable value.
// Used for change-detection to skip unchanged master entities.
func masterHash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum)
}

// filterChangedLedgers returns only those ledgers whose content hash has changed
// since the last sync, updating the store for unchanged ones implicitly (no-op).
func filterChangedLedgers(db *store.Store, ledgers []reader.Ledger) []reader.Ledger {
	var changed []reader.Ledger
	for _, l := range ledgers {
		h := masterHash(l)
		stored, _ := db.GetMasterHash("ledger", l.GUID)
		if stored != h {
			changed = append(changed, l)
		}
	}
	return changed
}

// filterChangedStockItems returns only stock items whose content hash has changed.
func filterChangedStockItems(db *store.Store, items []reader.StockItem) []reader.StockItem {
	var changed []reader.StockItem
	for _, it := range items {
		h := masterHash(it)
		stored, _ := db.GetMasterHash("stock_item", it.GUID)
		if stored != h {
			changed = append(changed, it)
		}
	}
	return changed
}

// filterChangedStockGroups returns only stock groups whose content hash has changed.
func filterChangedStockGroups(db *store.Store, groups []reader.StockGroup) []reader.StockGroup {
	var changed []reader.StockGroup
	for _, g := range groups {
		h := masterHash(g)
		stored, _ := db.GetMasterHash("stock_group", g.GUID)
		if stored != h {
			changed = append(changed, g)
		}
	}
	return changed
}

// filterChangedUOM returns only UOM records whose content hash has changed.
func filterChangedUOM(db *store.Store, uoms []reader.UOM) []reader.UOM {
	var changed []reader.UOM
	for _, u := range uoms {
		h := masterHash(u)
		stored, _ := db.GetMasterHash("uom", u.GUID)
		if stored != h {
			changed = append(changed, u)
		}
	}
	return changed
}

// filterChangedGodowns returns only godowns whose content hash has changed.
func filterChangedGodowns(db *store.Store, godowns []reader.Godown) []reader.Godown {
	var changed []reader.Godown
	for _, g := range godowns {
		h := masterHash(g)
		stored, _ := db.GetMasterHash("godown", g.GUID)
		if stored != h {
			changed = append(changed, g)
		}
	}
	return changed
}

// recordLedgerHashes persists content hashes for successfully synced ledgers.
func recordLedgerHashes(db *store.Store, ledgers []reader.Ledger) {
	for _, l := range ledgers {
		_ = db.SetMasterHash("ledger", l.GUID, masterHash(l))
	}
}

// recordStockItemHashes persists content hashes for successfully synced stock items.
func recordStockItemHashes(db *store.Store, items []reader.StockItem) {
	for _, it := range items {
		_ = db.SetMasterHash("stock_item", it.GUID, masterHash(it))
	}
}

// recordStockGroupHashes persists content hashes for successfully synced stock groups.
func recordStockGroupHashes(db *store.Store, groups []reader.StockGroup) {
	for _, g := range groups {
		_ = db.SetMasterHash("stock_group", g.GUID, masterHash(g))
	}
}

// recordUOMHashes persists content hashes for successfully synced UOM records.
func recordUOMHashes(db *store.Store, uoms []reader.UOM) {
	for _, u := range uoms {
		_ = db.SetMasterHash("uom", u.GUID, masterHash(u))
	}
}

// recordGodownHashes persists content hashes for successfully synced godowns.
func recordGodownHashes(db *store.Store, godowns []reader.Godown) {
	for _, g := range godowns {
		_ = db.SetMasterHash("godown", g.GUID, masterHash(g))
	}
}

// SyncBatch matches the TallySyncBatch interface in the cloud backend.
type SyncBatch struct {
	CompanyID     string                  `json:"companyId"`
	BatchID       string                  `json:"batchId"`
	FromTimestamp string                  `json:"fromTimestamp"`
	ToTimestamp   string                  `json:"toTimestamp"`
	Vouchers      []*reader.Voucher       `json:"vouchers,omitempty"`
	StockVouchers []*reader.StockVoucher  `json:"stockVouchers,omitempty"`
	Ledgers       []reader.Ledger         `json:"ledgers,omitempty"`
	StockItems    []reader.StockItem      `json:"stockItems,omitempty"`
	StockGroups   []reader.StockGroup     `json:"stockGroups,omitempty"`
	UOM           []reader.UOM            `json:"uom,omitempty"`
	Godowns       []reader.Godown         `json:"godowns,omitempty"`
	LedgerGroups  []reader.Group          `json:"ledgerGroups,omitempty"`
}

// DeltaSync performs one cycle of querying Tally for changes and sending them to the cloud.
func DeltaSync(ctx context.Context, client *tally.Client, db *store.Store, cloudURL string, machineToken string, companyID string, companyName string) error {
	// 1. Get last sync watermark from DB (empty = first sync, fetches all)
	lastSyncTS := db.GetLastSyncAt("vouchers")

	// 2. Load optional FY date range configured in the wizard (Step 3).
	//    Zero values fall back to the wide default inside ListDeltaVouchers.
	var fromDate, toDate time.Time
	if s := db.GetConfig("sync_from_date"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			fromDate = t
		}
	}
	if s := db.GetConfig("sync_to_date"); s != "" {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			toDate = t
		}
	}

	// For incremental syncs (watermark already set), extend toDate to today and
	// clamp fromDate to 3 months ago — the configured fromDate is only for the
	// first historical import. Keeping the full historical range would cause
	// 20+ monthly chunks to be scanned on every 5-minute poll (all returning 0).
	// 3 months is enough to catch any backdated voucher amendments.
	if lastSyncTS != "" {
		toDate = time.Now()
		threeMonthsAgo := toDate.AddDate(0, -3, 0)
		if fromDate.IsZero() || fromDate.Before(threeMonthsAgo) {
			fromDate = threeMonthsAgo
		}
	}

	// 3. Query Tally for vouchers altered since watermark
	vouchers, err := reader.ListDeltaVouchers(ctx, client, companyName, lastSyncTS, fromDate, toDate)
	if err != nil {
		return fmt.Errorf("ListDeltaVouchers: %w", err)
	}

	// Filter out draft (IsOptional), cancelled, and deleted vouchers.
	var active []reader.Voucher
	for _, v := range vouchers {
		if !v.IsOptional && !v.IsCancelled && !v.IsDeleted {
			active = append(active, v)
		}
	}

	// 3b. Fetch master data (ledgers + stock items + stock groups + UOM + godowns) —
	//     sent with the first batch so the cloud has party/item info before processing vouchers.
	var ledgers []reader.Ledger
	var stockItems []reader.StockItem
	var stockGroups []reader.StockGroup
	var uomList []reader.UOM
	var godowns []reader.Godown

	lastMasterSync := db.GetLastSyncAt("masters")
	if lastMasterSync == "" || lastSyncTS == "" {
		// First sync or never synced masters — fetch all
		fmt.Println("Fetching ledger masters from Tally...")
		if l, err := reader.ListLedgers(ctx, client, companyName); err == nil {
			ledgers = l
			fmt.Printf("Fetched %d ledgers\n", len(ledgers))
		} else {
			fmt.Printf("Warning: failed to fetch ledgers: %v\n", err)
		}

		fmt.Println("Fetching stock item masters from Tally...")
		if s, err := reader.ListStockItems(ctx, client, companyName, fromDate, toDate); err == nil {
			stockItems = s
			fmt.Printf("Fetched %d stock items\n", len(stockItems))
		} else {
			fmt.Printf("Warning: failed to fetch stock items: %v\n", err)
		}

		fmt.Println("Fetching stock group masters from Tally...")
		if g, err := reader.ListStockGroups(ctx, client, companyName); err == nil {
			stockGroups = g
			fmt.Printf("Fetched %d stock groups\n", len(stockGroups))
		} else {
			fmt.Printf("Warning: failed to fetch stock groups: %v\n", err)
		}

		fmt.Println("Fetching UOM masters from Tally...")
		if u, err := reader.ListUOM(ctx, client, companyName); err == nil {
			uomList = u
			fmt.Printf("Fetched %d UOM\n", len(uomList))
		} else {
			fmt.Printf("Warning: failed to fetch UOM: %v\n", err)
		}

		fmt.Println("Fetching godown masters from Tally...")
		if gd, err := reader.ListGodowns(ctx, client, companyName); err == nil {
			godowns = gd
			fmt.Printf("Fetched %d godowns\n", len(godowns))
		} else {
			fmt.Printf("Warning: failed to fetch godowns: %v\n", err)
		}
	}

	// Always fetch ledger groups (needed for ancestry chain — lightweight query)
	var ledgerGroups []reader.Group
	if lg, err := reader.ListGroups(ctx, client, companyName); err == nil {
		ledgerGroups = lg
		fmt.Printf("Fetched %d ledger groups\n", len(ledgerGroups))
	} else {
		fmt.Printf("Warning: failed to fetch ledger groups: %v\n", err)
	}

	if len(active) == 0 && len(ledgers) == 0 && len(stockItems) == 0 &&
		len(stockGroups) == 0 && len(uomList) == 0 && len(godowns) == 0 && len(ledgerGroups) == 0 {
		fmt.Printf("Delta sync: no new vouchers since %s\n", lastSyncTS)
		return nil
	}

	// 4. Apply hash-based incremental filtering — skip masters unchanged since last sync.
	ledgers = filterChangedLedgers(db, ledgers)
	stockItems = filterChangedStockItems(db, stockItems)
	stockGroups = filterChangedStockGroups(db, stockGroups)
	uomList = filterChangedUOM(db, uomList)
	godowns = filterChangedGodowns(db, godowns)

	// 5. Send master data to cloud first (so party addresses exist before voucher processing)
	now := time.Now()
	toTS := now.Format("20060102150405")

	if len(ledgers) > 0 {
		fmt.Printf("Uploading %d changed ledger masters to cloud...\n", len(ledgers))
		masterBatch := SyncBatch{
			CompanyID:     companyID,
			BatchID:       fmt.Sprintf("%d-masters-ledgers", now.UnixNano()),
			FromTimestamp: lastSyncTS,
			ToTimestamp:   toTS,
			Ledgers:       ledgers,
			LedgerGroups:  ledgerGroups,
		}
		if err := sendBatchToCloud(ctx, cloudURL, machineToken, masterBatch); err != nil {
			fmt.Printf("Warning: failed to send ledger masters: %v\n", err)
		} else {
			recordLedgerHashes(db, ledgers)
		}
	}

	if len(stockItems) > 0 {
		fmt.Printf("Uploading %d changed stock item masters to cloud...\n", len(stockItems))
		itemBatch := SyncBatch{
			CompanyID:     companyID,
			BatchID:       fmt.Sprintf("%d-masters-items", now.UnixNano()),
			FromTimestamp: lastSyncTS,
			ToTimestamp:   toTS,
			StockItems:    stockItems,
		}
		if err := sendBatchToCloud(ctx, cloudURL, machineToken, itemBatch); err != nil {
			fmt.Printf("Warning: failed to send stock item masters: %v\n", err)
		} else {
			recordStockItemHashes(db, stockItems)
		}
	}

	if len(stockGroups) > 0 {
		fmt.Printf("Uploading %d changed stock group masters to cloud...\n", len(stockGroups))
		sgBatch := SyncBatch{
			CompanyID:     companyID,
			BatchID:       fmt.Sprintf("%d-masters-stock-groups", now.UnixNano()),
			FromTimestamp: lastSyncTS,
			ToTimestamp:   toTS,
			StockGroups:   stockGroups,
		}
		if err := sendBatchToCloud(ctx, cloudURL, machineToken, sgBatch); err != nil {
			fmt.Printf("Warning: failed to send stock group masters: %v\n", err)
		} else {
			recordStockGroupHashes(db, stockGroups)
		}
	}

	if len(uomList) > 0 {
		fmt.Printf("Uploading %d changed UOM masters to cloud...\n", len(uomList))
		uomBatch := SyncBatch{
			CompanyID:     companyID,
			BatchID:       fmt.Sprintf("%d-masters-uom", now.UnixNano()),
			FromTimestamp: lastSyncTS,
			ToTimestamp:   toTS,
			UOM:           uomList,
		}
		if err := sendBatchToCloud(ctx, cloudURL, machineToken, uomBatch); err != nil {
			fmt.Printf("Warning: failed to send UOM masters: %v\n", err)
		} else {
			recordUOMHashes(db, uomList)
		}
	}

	if len(godowns) > 0 {
		fmt.Printf("Uploading %d changed godown masters to cloud...\n", len(godowns))
		gdBatch := SyncBatch{
			CompanyID:     companyID,
			BatchID:       fmt.Sprintf("%d-masters-godowns", now.UnixNano()),
			FromTimestamp: lastSyncTS,
			ToTimestamp:   toTS,
			Godowns:       godowns,
		}
		if err := sendBatchToCloud(ctx, cloudURL, machineToken, gdBatch); err != nil {
			fmt.Printf("Warning: failed to send godown masters: %v\n", err)
		} else {
			recordGodownHashes(db, godowns)
		}
	}

	// Update master sync watermark
	if len(ledgers) > 0 || len(stockItems) > 0 || len(stockGroups) > 0 || len(uomList) > 0 || len(godowns) > 0 {
		if err := db.SetLastSyncAt("masters", toTS); err != nil {
			fmt.Printf("Warning: failed to update masters sync watermark: %v\n", err)
		}
	}

	if len(active) == 0 {
		fmt.Printf("Delta sync: pushed %d ledgers, %d items, %d stock groups, %d UOM, %d godowns (no new vouchers)\n",
			len(ledgers), len(stockItems), len(stockGroups), len(uomList), len(godowns))
		return nil
	}

	// 5. Fetch and send stock vouchers in the same date range.
	fmt.Println("Fetching stock vouchers from Tally...")
	stockVouchers, err := reader.FetchStockVouchers(ctx, client, companyName, lastSyncTS, fromDate, toDate)
	if err != nil {
		fmt.Printf("Warning: failed to fetch stock vouchers: %v\n", err)
	}
	// Filter out optional/cancelled/deleted stock vouchers.
	var activeStockVouchers []reader.StockVoucher
	for _, sv := range stockVouchers {
		if !sv.IsOptional && !sv.IsCancelled && !sv.IsDeleted {
			activeStockVouchers = append(activeStockVouchers, sv)
		}
	}
	if len(activeStockVouchers) > 0 {
		fmt.Printf("Uploading %d stock vouchers to cloud...\n", len(activeStockVouchers))
		var svPtrs []*reader.StockVoucher
		for i := range activeStockVouchers {
			svPtrs = append(svPtrs, &activeStockVouchers[i])
		}
		svBatch := SyncBatch{
			CompanyID:     companyID,
			BatchID:       fmt.Sprintf("%d-stock-vouchers", now.UnixNano()),
			FromTimestamp: lastSyncTS,
			ToTimestamp:   toTS,
			StockVouchers: svPtrs,
		}
		if err := sendBatchToCloud(ctx, cloudURL, machineToken, svBatch); err != nil {
			fmt.Printf("Warning: failed to send stock vouchers: %v\n", err)
		}
	}

	// 6. Send accounting vouchers to cloud in batches of 500 to avoid huge payloads
	const batchSize = 500
	totalSent := 0

	for start := 0; start < len(active); start += batchSize {
		end := start + batchSize
		if end > len(active) {
			end = len(active)
		}

		var voucherPtrs []*reader.Voucher
		for i := start; i < end; i++ {
			voucherPtrs = append(voucherPtrs, &active[i])
		}

		batch := SyncBatch{
			CompanyID:     companyID,
			BatchID:       fmt.Sprintf("%d-%d", now.UnixNano(), start),
			FromTimestamp: lastSyncTS,
			ToTimestamp:   toTS,
			Vouchers:      voucherPtrs,
		}

		fmt.Printf("Uploading batch %d–%d of %d vouchers to cloud...\n", start+1, end, len(active))
		if err := sendBatchToCloud(ctx, cloudURL, machineToken, batch); err != nil {
			return fmt.Errorf("failed to send batch %d–%d to cloud: %w", start+1, end, err)
		}
		totalSent += len(voucherPtrs)
	}

	// Advance watermark — only on success
	if err := db.SetLastSyncAt("vouchers", toTS); err != nil {
		fmt.Printf("Warning: failed to update sync watermark: %v\n", err)
	}

	fmt.Printf("Delta sync: pushed %d vouchers to cloud in %d batches (from=%s to=%s)\n",
		totalSent, (len(active)+batchSize-1)/batchSize, lastSyncTS, toTS)
	return nil
}

func sendBatchToCloud(ctx context.Context, cloudURL, token string, batch SyncBatch) error {
	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cloudURL+"/connector/sync-data", bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cloud rejected sync data: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

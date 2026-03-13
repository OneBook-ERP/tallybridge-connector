package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tallybridge/connector/store"
	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/reader"
)

// SyncBatch matches the TallySyncBatch interface in the cloud backend.
type SyncBatch struct {
	CompanyID     string            `json:"companyId"`
	BatchID       string            `json:"batchId"`
	FromTimestamp string            `json:"fromTimestamp"`
	ToTimestamp   string            `json:"toTimestamp"`
	Vouchers      []*reader.Voucher `json:"vouchers,omitempty"`
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

	// 3. Query Tally for vouchers altered since watermark
	vouchers, err := reader.ListDeltaVouchers(ctx, client, companyName, lastSyncTS, fromDate, toDate)
	if err != nil {
		return fmt.Errorf("ListDeltaVouchers: %w", err)
	}

	// Filter optional (draft) vouchers — PRD edge case #1
	var active []reader.Voucher
	for _, v := range vouchers {
		if !v.IsOptional {
			active = append(active, v)
		}
	}

	if len(active) == 0 {
		fmt.Printf("Delta sync: no new vouchers since %s\n", lastSyncTS)
		return nil
	}

	// 4. Send vouchers to cloud in batches of 500 to avoid huge payloads
	now := time.Now()
	toTS := now.Format("20060102150405")

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

	// 5. Advance watermark — only on success
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

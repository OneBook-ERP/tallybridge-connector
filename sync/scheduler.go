package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/tallybridge/connector/store"
	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/reader"
)

// Scheduler runs the periodic delta sync cycle (Step 19).
type Scheduler struct {
	client       *tally.Client
	db           *store.Store
	cloudURL     string
	machineToken string
	companyID    string
	companyName  string
	interval     time.Duration
	stopCh       chan struct{}
	stopOnce     sync.Once
}

// NewScheduler creates a new background sync scheduler.
func NewScheduler(client *tally.Client, db *store.Store, cloudURL, token, companyID, companyName string, interval time.Duration) *Scheduler {
	return &Scheduler{
		client:       client,
		db:           db,
		cloudURL:     cloudURL,
		machineToken: token,
		companyID:    companyID,
		companyName:  companyName,
		interval:     interval,
		stopCh:       make(chan struct{}),
	}
}

// sendHeartbeat posts the connector's status to the cloud.
// tallyCounts may be nil for lightweight heartbeats (sent every 30s);
// it is populated only on the sync ticker (every 5 min) to avoid blocking.
func (s *Scheduler) sendHeartbeat(tallyCounts map[string]interface{}) {
	tallyStatus := "OFFLINE"
	if s.client != nil {
		tallyStatus = "ONLINE"
	}

	payload := map[string]interface{}{
		"companyId":        s.companyID,
		"tallyStatus":      tallyStatus,
		"openCompany":      s.companyName,
		"pendingLocalJobs": 0,
	}
	if len(tallyCounts) > 0 {
		payload["tallyCounts"] = tallyCounts
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", s.cloudURL+"/connector/heartbeat", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Heartbeat error (build request): %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.machineToken)

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		fmt.Printf("Heartbeat error (send): %v\n", err)
		return
	}
	resp.Body.Close()
}

// fetchTallyCounts queries Tally for voucher and master counts.
// Runs in the sync ticker goroutine (every 5 min) — never in the 30s heartbeat.
func (s *Scheduler) fetchTallyCounts() map[string]interface{} {
	if s.client == nil || s.db == nil {
		return nil
	}
	tallyURL := s.db.GetConfig("tally_url")
	fromDate := s.db.GetConfig("sync_from_date")
	toDate := s.db.GetConfig("sync_to_date")
	if tallyURL == "" || s.companyName == "" {
		return nil
	}
	counts := make(map[string]interface{})
	if vc, err := reader.GetVoucherCounts(tallyURL, s.companyName, fromDate, toDate); err == nil {
		for k, v := range vc {
			counts[k] = v
		}
	}
	if mc, err := reader.GetMasterCounts(tallyURL, s.companyName); err == nil {
		for k, v := range mc {
			counts[k] = v
		}
	}
	return counts
}

// Start begins the delta sync ticker and the reverse sync long-poll loop.
func (s *Scheduler) Start() {
	// Send initial heartbeat immediately (no counts — don't block startup).
	go s.sendHeartbeat(nil)

	// Tally → ERPNext: periodic delta sync + heartbeat
	go func() {
		fmt.Println("Starting Delta Sync Scheduler (interval:", s.interval, ")")
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		hbTicker := time.NewTicker(30 * time.Second)
		defer hbTicker.Stop()

		for {
			select {
			case <-ticker.C:
				// Skip sync if disabled via the UI toggle; still send heartbeat.
				if s.db.GetConfig("sync_enabled") == "false" {
					fmt.Println("[Scheduler] Auto sync is disabled — skipping this cycle")
					s.sendHeartbeat(nil)
					continue
				}
				// Fetch counts once per sync cycle and include in heartbeat.
				counts := s.fetchTallyCounts()
				s.sendHeartbeat(counts)
				err := DeltaSync(context.Background(), s.client, s.db, s.cloudURL, s.machineToken, s.companyID, s.companyName)
				if err != nil {
					fmt.Printf("Delta sync error: %v\n", err)
				} else {
					fmt.Println("Delta sync completed successfully")
				}
			case <-hbTicker.C:
				// Lightweight heartbeat — no counts to avoid blocking.
				s.sendHeartbeat(nil)
			case <-s.stopCh:
				fmt.Println("Stopping Delta Sync Scheduler")
				return
			}
		}
	}()

	// ERPNext → Tally: long-poll loop for reverse jobs
	go func() {
		fmt.Println("Starting Reverse Sync (long-poll) loop")
		ctx, cancel := context.WithCancel(context.Background())
		go func() { <-s.stopCh; cancel() }()
		PollAndWriteJobs(ctx, s.client, s.db, s.cloudURL, s.machineToken, s.companyID)
		fmt.Println("Reverse Sync loop stopped")
	}()
}

// Stop halts the scheduler. Safe to call multiple times.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

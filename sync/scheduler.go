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
func (s *Scheduler) sendHeartbeat() {
	tallyStatus := "OFFLINE"
	if s.client != nil {
		tallyStatus = "ONLINE"
	}

	body, _ := json.Marshal(map[string]interface{}{
		"companyId":        s.companyID,
		"tallyStatus":      tallyStatus,
		"openCompany":      s.companyName,
		"pendingLocalJobs": 0,
	})

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

// Start begins the delta sync ticker and the reverse sync long-poll loop.
func (s *Scheduler) Start() {
	// Send initial heartbeat immediately
	go s.sendHeartbeat()

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
				s.sendHeartbeat()
				err := DeltaSync(context.Background(), s.client, s.db, s.cloudURL, s.machineToken, s.companyID, s.companyName)
				if err != nil {
					fmt.Printf("Delta sync error: %v\n", err)
				} else {
					fmt.Println("Delta sync completed successfully")
				}
			case <-hbTicker.C:
				s.sendHeartbeat()
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

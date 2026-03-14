package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tallybridge/connector/store"
	"github.com/tallybridge/connector/sync"
	"github.com/tallybridge/connector/tally"
)

func main() {
	db, err := store.InitDB("connector.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open DB: %v\n", err)
		os.Exit(1)
	}

	tallyURL := db.GetConfig("tally_url")
	if tallyURL == "" {
		tallyURL = "http://192.168.1.12:9000"
	}
	cloudURL := db.GetConfig("cloud_url")
	machineToken := db.GetConfig("machine_token")
	companyID := db.GetConfig("company_id")
	companyName := db.GetConfig("company_tally_name")

	fmt.Printf("Tally: %s\nCloud: %s\nCompany: %s (%s)\n", tallyURL, cloudURL, companyName, companyID)

	client := tally.NewClient(tally.WithAddr(tallyURL))
	ctx := context.Background()

	err = sync.DeltaSync(ctx, client, db, cloudURL, machineToken, companyID, companyName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sync failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Sync complete!")
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	tbsync "github.com/tallybridge/connector/sync"
	"github.com/tallybridge/connector/store"
	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/reader"
)

// erpCall is a thin helper for authenticated ERPNext REST calls.
// method: "GET" | "POST" | "PUT". body may be nil for GET.
func (a *App) erpCall(method, path string, body interface{}) ([]byte, int, error) {
	erpURL := a.db.GetConfig("erpnext_url")
	apiKey := a.db.GetConfig("erpnext_api_key")
	apiSecret := a.db.GetConfig("erpnext_api_secret")
	if erpURL == "" {
		return nil, 0, fmt.Errorf("ERPNext URL not configured")
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(a.ctx, method, erpURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("token %s:%s", apiKey, apiSecret))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	cl := &http.Client{Timeout: 15 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// Version is set at build time via -ldflags "-X main.Version=x.y.z". Defaults to "dev".
var Version = "dev"

// App struct
type App struct {
	ctx         context.Context
	db          *store.Store
	tallyClient *tally.Client
	scheduler   *tbsync.Scheduler
}

// NewApp creates a new App application struct
func NewApp(db *store.Store, tallyClient *tally.Client) *App {
	return &App{
		db:          db,
		tallyClient: tallyClient,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ConnectTally updates the client's address and tests the connection
func (a *App) ConnectTally(url string) map[string]interface{} {
	a.tallyClient = tally.NewClient(tally.WithAddr(url))
	return a.ScanTally()
}

// ScanTally attempts to connect to the Tally ERP 9000 port locally
func (a *App) ScanTally() map[string]interface{} {
	if a.tallyClient == nil {
		return map[string]interface{}{"status": "error", "message": "Tally client not configured"}
	}
	info := reader.Heartbeat(a.ctx, a.tallyClient)
	if !info.Online {
		return map[string]interface{}{"status": "error", "message": "Tally not reachable on port 9000"}
	}

	return map[string]interface{}{
		"status":  "ok",
		"online":  info.Online,
		"company": info.OpenCompany,
	}
}

// GetCompanies lists all companies currently open in Tally
func (a *App) GetCompanies() []string {
	var results []string
	if a.tallyClient == nil {
		return results
	}
	comps, err := reader.ListCompanies(a.ctx, a.tallyClient)
	if err != nil {
		return results
	}
	for _, c := range comps {
		results = append(results, c.Name)
	}
	return results
}

// SaveConfig saves the ERPNext and cloud configuration details
func (a *App) SaveConfig(erpUrl, apiKey, apiSecret, company, cloudURL string) error {
	_ = a.db.SetConfig("erpnext_url", erpUrl)
	_ = a.db.SetConfig("erpnext_api_key", apiKey)
	_ = a.db.SetConfig("erpnext_api_secret", apiSecret)
	_ = a.db.SetConfig("company_tally_name", company)
	if cloudURL != "" {
		_ = a.db.SetConfig("cloud_url", cloudURL)
	}
	return nil
}

// SaveERPNextSettings persists additional ERPNext configuration (GSTIN, warehouse, cost center).
// These are optional but needed for opening stock, journal entries, and GST inter-state detection.
func (a *App) SaveERPNextSettings(gstin, warehouse, costCenter string) error {
	if gstin != "" {
		_ = a.db.SetConfig("erpnext_gstin", gstin)
	}
	if warehouse != "" {
		_ = a.db.SetConfig("erpnext_warehouse", warehouse)
	}
	if costCenter != "" {
		_ = a.db.SetConfig("erpnext_cost_center", costCenter)
	}
	return nil
}

// SetupERPNextWebhooks creates TallyBridge webhooks in ERPNext for the on_submit and
// on_cancel events of Sales Invoice, Purchase Invoice, Payment Entry, and Journal Entry.
// Webhooks post to {cloudUrl}/webhook/erpnext. Skips doctypes that already have a
// TallyBridge webhook. Returns counts of created, skipped, and errored webhooks.
func (a *App) SetupERPNextWebhooks() map[string]interface{} {
	cloudURL := a.db.GetConfig("cloud_url")
	if cloudURL == "" {
		return map[string]interface{}{"status": "error", "message": "Cloud URL not configured — complete setup first"}
	}
	webhookURL := strings.TrimRight(cloudURL, "/") + "/webhook/erpnext"

	type webhookSpec struct {
		Doctype string
		Event   string
	}
	specs := []webhookSpec{
		{"Sales Invoice", "on_submit"},
		{"Sales Invoice", "on_cancel"},
		{"Purchase Invoice", "on_submit"},
		{"Purchase Invoice", "on_cancel"},
		{"Payment Entry", "on_submit"},
		{"Payment Entry", "on_cancel"},
		{"Journal Entry", "on_submit"},
		{"Journal Entry", "on_cancel"},
	}

	created, skipped, errCount := 0, 0, 0
	var errMsgs []string

	for _, spec := range specs {
		// Check if a webhook for this doctype+event already points to our URL
		checkPath := fmt.Sprintf(
			`/api/resource/Webhook?filters=[[%%22webhook_doctype%%22,%%22=%%22,%%22%s%%22],[%%22webhook_docevent%%22,%%22=%%22,%%22%s%%22],[%%22request_url%%22,%%22like%%22,%%22%%25/webhook/erpnext%%25%%22]]&fields=[%%22name%%22]&limit=1`,
			url.QueryEscape(spec.Doctype), url.QueryEscape(spec.Event),
		)
		existData, existStatus, _ := a.erpCall("GET", checkPath, nil)
		if existStatus == 200 {
			var existResp struct {
				Data []struct{ Name string `json:"name"` } `json:"data"`
			}
			if json.Unmarshal(existData, &existResp) == nil && len(existResp.Data) > 0 {
				skipped++
				continue
			}
		}

		webhookName := fmt.Sprintf("TALLYBRIDGE-%s-%s", strings.ReplaceAll(spec.Doctype, " ", "-"), spec.Event)
		body := map[string]interface{}{
			"doctype":           "Webhook",
			"name":              webhookName,
			"webhook_doctype":   spec.Doctype,
			"webhook_docevent":  spec.Event,
			"request_url":       webhookURL,
			"request_structure": "Form URL-Encoded",
			"enabled":           1,
			// Use .get() to avoid AttributeError if custom field missing during validation
			"condition": "doc.get('custom_sync_source') != 'TALLYBRIDGE'",
		}
		respData, status, err := a.erpCall("POST", "/api/resource/Webhook", body)
		switch {
		case err != nil:
			errCount++
			errMsgs = append(errMsgs, fmt.Sprintf("%s %s: %v", spec.Doctype, spec.Event, err))
		case status == 200 || status == 201:
			created++
		default:
			errCount++
			snippet := string(respData)
			if len(snippet) > 200 {
				snippet = snippet[:200]
			}
			errMsgs = append(errMsgs, fmt.Sprintf("%s %s: HTTP %d — %s", spec.Doctype, spec.Event, status, snippet))
		}
	}

	status := "ok"
	if errCount > 0 && created == 0 {
		status = "error"
	} else if errCount > 0 {
		status = "partial"
	}
	return map[string]interface{}{
		"status":  status,
		"created": created,
		"skipped": skipped,
		"errors":  errCount,
		"details": strings.Join(errMsgs, "\n"),
	}
}

// TestERPNextConnection tests the ERPNext API credentials locally from the connector
func (a *App) TestERPNextConnection(erpUrl, apiKey, apiSecret string) map[string]interface{} {
	url := fmt.Sprintf("%s/api/method/frappe.auth.get_logged_user", erpUrl)
	req, err := http.NewRequestWithContext(a.ctx, "GET", url, nil)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": "Failed to create request: " + err.Error()}
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s:%s", apiKey, apiSecret))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": "Network error: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return map[string]interface{}{"status": "error", "message": fmt.Sprintf("ERPNext returned HTTP %d", resp.StatusCode)}
	}

	return map[string]interface{}{"status": "ok", "message": "Connection successful"}
}

// ValidateERPNextRequirements checks ERPNext for required apps, custom fields, and tax templates.
func (a *App) ValidateERPNextRequirements() []map[string]string {
	checks := []map[string]string{}
	add := func(label, status string) {
		checks = append(checks, map[string]string{"label": label, "status": status})
	}

	// India Compliance
	_, icStatus, _ := a.erpCall("GET", `/api/resource/DocType/GST Settings`, nil)
	if icStatus == 200 || icStatus == 403 {
		add("India Compliance App Installed", "ok")
	} else {
		add("India Compliance App Installed", "warn")
	}

	// Chart of Accounts
	erpCompany := a.db.GetConfig("erpnext_company")
	if erpCompany != "" {
		coaData, coaStatus, _ := a.erpCall("GET", fmt.Sprintf(`/api/resource/Company/%s?fields=["chart_of_accounts"]`, erpCompany), nil)
		if coaStatus == 200 {
			var coaResp struct {
				Data struct{ ChartOfAccounts string `json:"chart_of_accounts"` } `json:"data"`
			}
			if json.Unmarshal(coaData, &coaResp) == nil && coaResp.Data.ChartOfAccounts != "" {
				add("Chart of Accounts configured", "ok")
			} else {
				add("Chart of Accounts configured", "warn")
			}
		} else {
			add("Chart of Accounts configured", "warn")
		}
	} else {
		add("Chart of Accounts configured", "warn")
	}

	// Taxes and Charges Templates
	taxData, taxStatus, _ := a.erpCall("GET", `/api/resource/Sales Taxes and Charges Template?limit=1`, nil)
	if taxStatus == 200 {
		var taxResp struct{ Data []interface{} `json:"data"` }
		if json.Unmarshal(taxData, &taxResp) == nil && len(taxResp.Data) > 0 {
			add("Taxes and Charges Templates exist", "ok")
		} else {
			add("Taxes and Charges Templates exist", "warn")
		}
	} else {
		add("Taxes and Charges Templates exist", "warn")
	}

	// TallyBridge Custom Fields
	cfData, cfStatus, _ := a.erpCall("GET", `/api/resource/Custom Field?filters=[["dt","=","Sales Invoice"],["fieldname","=","custom_tally_guid"]]&limit=1`, nil)
	if cfStatus == 200 {
		var cfResp struct{ Data []struct{ Name string `json:"name"` } `json:"data"` }
		if json.Unmarshal(cfData, &cfResp) == nil && len(cfResp.Data) > 0 {
			add("TallyBridge Custom Fields exist", "ok")
		} else {
			add("TallyBridge Custom Fields exist", "warn")
		}
	} else {
		add("TallyBridge Custom Fields exist", "warn")
	}

	return checks
}

// DataQualityScan checks Tally data quality across ledgers and stock items.
// Returns issue counts broken down by category.
func (a *App) DataQualityScan(company string) map[string]interface{} {
	result := map[string]interface{}{
		"ledgerTotal":     0,
		"missingGSTIN":    0,
		"stockItemTotal":  0,
		"missingUOM":      0,
		"missingHSN":      0,
		"warnCount":       0,
		"readyCount":      0,
	}
	if a.tallyClient == nil || company == "" {
		return result
	}

	ledgers, err := reader.ListLedgers(a.ctx, a.tallyClient, company)
	if err != nil {
		fmt.Printf("[DataQualityScan] ListLedgers error: %v\n", err)
	} else {
		result["ledgerTotal"] = len(ledgers)
		missingGSTIN := 0
		for _, l := range ledgers {
			// Use GSTRegistrationType for accurate party identification.
			// "Regular" and "Composition" are registered GST parties that MUST have GSTIN.
			isGSTParty := l.GSTRegistrationType == "Regular" || l.GSTRegistrationType == "Composition"
			if isGSTParty && l.PartyGSTIN == "" {
				missingGSTIN++
			}
		}
		result["missingGSTIN"] = missingGSTIN
	}

	items, err := reader.ListStockItems(a.ctx, a.tallyClient, company, time.Time{}, time.Time{})
	if err != nil {
		fmt.Printf("[DataQualityScan] ListStockItems error: %v\n", err)
	} else {
		result["stockItemTotal"] = len(items)
		missingUOM, missingHSN := 0, 0
		for _, it := range items {
			if it.BaseUnits == "" {
				missingUOM++
			}
			// TaxClassificationName is a fallback — if set, the item is linked
			// to a tax classification that carries the HSN, so it's not missing.
			if it.HSNCode == "" && it.TaxClassificationName == "" {
				missingHSN++
			}
		}
		result["missingUOM"] = missingUOM
		result["missingHSN"] = missingHSN
	}

	// Total warnings = all issues summed
	warnCount := toInt(result["missingGSTIN"]) + toInt(result["missingUOM"]) + toInt(result["missingHSN"])
	result["warnCount"] = warnCount
	result["readyCount"] = toInt(result["ledgerTotal"]) + toInt(result["stockItemTotal"]) - warnCount

	return result
}

func toInt(v interface{}) int {
	if n, ok := v.(int); ok {
		return n
	}
	return 0
}

// SaveSyncRange persists the financial year date range for the selected company.
// fromDate and toDate must be "YYYY-MM-DD" (e.g. "2023-04-01", "2024-03-31").
func (a *App) SaveSyncRange(fromDate, toDate string) error {
	if err := a.db.SetConfig("sync_from_date", fromDate); err != nil {
		return err
	}
	return a.db.SetConfig("sync_to_date", toDate)
}

// GetMappingConfig returns the JSON config string for a mapping section for the current company.
func (a *App) GetMappingConfig(section string) string {
	companyID := a.db.GetConfig("company_id")
	return a.db.GetMappingConfig(companyID, section)
}

// SetMappingConfig saves the JSON config for a mapping section for the current company.
func (a *App) SetMappingConfig(section, jsonConfig string) error {
	companyID := a.db.GetConfig("company_id")
	return a.db.SetMappingConfig(companyID, section, jsonConfig)
}

// GetAllMappingConfigs returns all mapping sections for the current company.
func (a *App) GetAllMappingConfigs() map[string]string {
	companyID := a.db.GetConfig("company_id")
	return a.db.GetAllMappingConfigs(companyID)
}

// RegisterWithCloud registers this connector with the cloud middleware.
// It POSTs to /connector/register, stores the returned machineToken and companyId
// in SQLite, and returns status to the UI.
func (a *App) RegisterWithCloud(cloudURL, companyName string) map[string]interface{} {
	// Get or generate a stable machine ID
	machineID := a.db.GetConfig("machine_id")
	if machineID == "" {
		hostname, _ := os.Hostname()
		machineID = fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
		_ = a.db.SetConfig("machine_id", machineID)
	}

	// If the company name has changed, clear the old company's cached mappings
	// and credentials so the new company starts fresh.
	previousCompany := a.db.GetConfig("company_tally_name")
	if previousCompany != "" && previousCompany != companyName {
		oldCompanyID := a.db.GetConfig("company_id")
		if oldCompanyID != "" {
			_ = a.db.ClearMappingConfig(oldCompanyID)
		}
		_ = a.db.SetConfig("machine_token", "")
		_ = a.db.SetConfig("company_id", "")
		fmt.Printf("[RegisterWithCloud] Company changed from %q to %q — cleared old state\n", previousCompany, companyName)
	}

	// Cloud is the source of truth for all mappings.
	// Registration only sends identity — no local mapping data.
	body := map[string]interface{}{
		"machineId":   machineID,
		"companyName": companyName,
		"osInfo":      runtime.GOOS + "/" + runtime.GOARCH,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": "Failed to encode request: " + err.Error()}
	}

	req, err := http.NewRequestWithContext(a.ctx, "POST", cloudURL+"/connector/register", bytes.NewReader(payload))
	if err != nil {
		return map[string]interface{}{"status": "error", "message": "Failed to create request: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": "Cloud unreachable: " + err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Cloud returned HTTP %d: %s", resp.StatusCode, string(respBody)),
		}
	}

	var result struct {
		MachineToken string `json:"machineToken"`
		CompanyID    string `json:"companyId"`
		CloudConfig  struct {
			PollIntervalMinutes int `json:"pollIntervalMinutes"`
		} `json:"cloudConfig"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return map[string]interface{}{"status": "error", "message": "Failed to parse cloud response: " + err.Error()}
	}

	// Persist registration credentials
	_ = a.db.SetConfig("machine_token", result.MachineToken)
	_ = a.db.SetConfig("company_id", result.CompanyID)
	_ = a.db.SetConfig("cloud_url", cloudURL)
	_ = a.db.SetConfig("company_tally_name", companyName)
	_ = a.db.SetConfig("poll_interval_minutes", fmt.Sprintf("%d", result.CloudConfig.PollIntervalMinutes))
	_ = a.db.SetConfig("last_registered_at", time.Now().Format(time.RFC3339))

	fmt.Printf("Registered with cloud: companyId=%s\n", result.CompanyID)

	// Pull cloud-seeded defaults (GST mappings, voucher types) into local cache.
	// This is a best-effort call — if it fails the next sync will retry.
	go func() {
		if r := a.GetCloudConfig(); r["status"] != "ok" {
			fmt.Printf("[RegisterWithCloud] GetCloudConfig after registration: %v\n", r["message"])
		}
	}()

	return map[string]interface{}{
		"status":    "ok",
		"companyId": result.CompanyID,
		"message":   "Registered successfully",
	}
}

// GetCloudConfig pulls the latest config from the cloud platform and stores
// account/GST/voucher-type/godown mappings into the local mapping_config SQLite table.
func (a *App) GetCloudConfig() map[string]interface{} {
	data, status, err := a.cloudCall("GET", "/connector/config", nil)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": err.Error()}
	}
	if status != 200 {
		return map[string]interface{}{"status": "error", "message": fmt.Sprintf("cloud returned %d", status)}
	}
	var resp struct {
		AccountMappings         map[string]interface{} `json:"accountMappings"`
		GstLedgerMappings       map[string]interface{} `json:"gstLedgerMappings"`
		VoucherTypeMappings     map[string]interface{} `json:"voucherTypeMappings"`
		GodownWarehouseMappings map[string]interface{} `json:"godownWarehouseMappings"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return map[string]interface{}{"status": "error", "message": "parse error: " + err.Error()}
	}
	// Cache each section into the local mapping_config table scoped by company.
	companyID := a.db.GetConfig("company_id")
	if resp.AccountMappings != nil {
		if b, err := json.Marshal(resp.AccountMappings); err == nil {
			_ = a.db.SetMappingConfig(companyID, "account_coa", string(b))
		}
	}
	if resp.GstLedgerMappings != nil {
		if b, err := json.Marshal(resp.GstLedgerMappings); err == nil {
			_ = a.db.SetMappingConfig(companyID, "gst_ledger", string(b))
		}
	}
	if resp.VoucherTypeMappings != nil {
		if b, err := json.Marshal(resp.VoucherTypeMappings); err == nil {
			_ = a.db.SetMappingConfig(companyID, "voucher_types", string(b))
		}
	}
	if resp.GodownWarehouseMappings != nil {
		if b, err := json.Marshal(resp.GodownWarehouseMappings); err == nil {
			_ = a.db.SetMappingConfig(companyID, "godown_warehouse", string(b))
		}
	}
	return map[string]interface{}{"status": "ok", "message": "Config pulled from cloud"}
}

// StartBackgroundSync reads registration credentials from SQLite and starts
// the periodic delta sync scheduler.
func (a *App) StartBackgroundSync() map[string]interface{} {
	cloudURL := a.db.GetConfig("cloud_url")
	token := a.db.GetConfig("machine_token")
	companyID := a.db.GetConfig("company_id")
	companyName := a.db.GetConfig("company_tally_name")

	if cloudURL == "" || token == "" || companyID == "" {
		return map[string]interface{}{
			"status":  "error",
			"message": "Not registered with cloud — complete registration first",
		}
	}

	// Push ERPNext credentials first so the cloud can seed defaults,
	// then pull the seeded config into local cache.
	if err := a.PushMappingsToCloud(); err != nil {
		fmt.Printf("[StartBackgroundSync] Warning: could not push credentials: %v\n", err)
	}
	a.GetCloudConfig()

	// Stop any existing scheduler before replacing
	if a.scheduler != nil {
		a.scheduler.Stop()
	}

	interval := 5 * time.Minute
	a.scheduler = tbsync.NewScheduler(a.tallyClient, a.db, cloudURL, token, companyID, companyName, interval)
	a.scheduler.Start()

	fmt.Printf("Background sync started (companyId=%s, interval=%s)\n", companyID, interval)
	return map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Background sync active (every %s)", interval),
	}
}

// ─── Tally data accessors ────────────────────────────────────────────────────

// GetCompanyStats returns ledger, group, and voucher-type counts for a Tally company.
// company must be the full decorated name as returned by ListCompanies
// (e.g. "COVAI POLYMER TRADERS - (2023-24)") — Tally requires the FY suffix for
// company-specific collection queries; bare names return 0 records.
func (a *App) GetCompanyStats(company string) map[string]interface{} {
	result := map[string]interface{}{
		"ledgerCount":      0,
		"groupCount":       0,
		"voucherTypeCount": 0,
		"voucherCount":     0,
	}
	if a.tallyClient == nil {
		return result
	}
	if ledgers, err := reader.ListLedgers(a.ctx, a.tallyClient, company); err == nil {
		result["ledgerCount"] = len(ledgers)
	} else {
		fmt.Printf("[GetCompanyStats] ListLedgers error: %v\n", err)
	}
	if groups, err := reader.ListGroups(a.ctx, a.tallyClient, company); err == nil {
		result["groupCount"] = len(groups)
	} else {
		fmt.Printf("[GetCompanyStats] ListGroups error: %v\n", err)
	}
	if vtypes, err := reader.ListVoucherTypes(a.ctx, a.tallyClient, company); err == nil {
		result["voucherTypeCount"] = len(vtypes)
	} else {
		fmt.Printf("[GetCompanyStats] ListVoucherTypes error: %v\n", err)
	}
	// Derive FY date range from company name pattern "(YYYY-YY)"
	fromDate, toDate := parseFYFromCompanyName(company)
	if n, err := reader.CountVouchers(a.ctx, a.tallyClient, company, fromDate, toDate); err == nil {
		result["voucherCount"] = n
	} else {
		fmt.Printf("[GetCompanyStats] CountVouchers error: %v\n", err)
	}
	return result
}

// parseFYFromCompanyName extracts the financial year date range from a Tally company name.
// Example: "COVAI POLYMER TRADERS - (2023-24)" → 2023-04-01, 2024-03-31.
// Falls back to a wide range if the pattern is not found.
func parseFYFromCompanyName(name string) (time.Time, time.Time) {
	// Look for "(YYYY-YY)" at the end of the company name
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '(' {
			tail := name[i+1:]
			var fromYear, toSuffix int
			if n, _ := fmt.Sscanf(tail, "%d-%d", &fromYear, &toSuffix); n == 2 && fromYear > 1990 && fromYear < 2100 {
				toYear := (fromYear/100)*100 + toSuffix
				if toYear <= fromYear {
					toYear += 100
				}
				from := time.Date(fromYear, 4, 1, 0, 0, 0, 0, time.UTC)
				to := time.Date(toYear, 3, 31, 0, 0, 0, 0, time.UTC)
				return from, to
			}
			break
		}
	}
	// Fallback: wide range
	return time.Date(2000, 4, 1, 0, 0, 0, 0, time.UTC), time.Now().UTC().AddDate(1, 0, 0)
}

// GetTallyLedgers returns a lightweight list of ledger names + parent groups.
func (a *App) GetTallyLedgers(company string) []map[string]interface{} {
	if a.tallyClient == nil {
		return nil
	}
	ledgers, err := reader.ListLedgers(a.ctx, a.tallyClient, company)
	if err != nil {
		fmt.Printf("[GetTallyLedgers] error: %v\n", err)
		return nil
	}
	out := make([]map[string]interface{}, 0, len(ledgers))
	for _, l := range ledgers {
		out = append(out, map[string]interface{}{
			"name":   l.Name,
			"parent": l.Parent,
			"gstin":  l.PartyGSTIN,
			"gstReg": l.GSTRegistrationType,
		})
	}
	return out
}

// GetTallyGroups returns full group hierarchy from Tally XML API.
func (a *App) GetTallyGroups(company string) []map[string]interface{} {
	if a.tallyClient == nil {
		return nil
	}
	groups, err := reader.ListGroups(a.ctx, a.tallyClient, company)
	if err != nil {
		fmt.Printf("[GetTallyGroups] error: %v\n", err)
		return nil
	}
	out := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		out = append(out, map[string]interface{}{
			"name":             g.Name,
			"parent":           g.Parent,
			"primaryGroup":     g.PrimaryGroup,
			"isRevenue":        g.IsRevenue,
			"isDeemedPositive": g.IsDeemedPositive,
		})
	}
	return out
}

// GetTallyVoucherTypes returns voucher type name + nature for mapping.
func (a *App) GetTallyVoucherTypes(company string) []map[string]interface{} {
	if a.tallyClient == nil {
		return nil
	}
	vtypes, err := reader.ListVoucherTypes(a.ctx, a.tallyClient, company)
	if err != nil {
		return nil
	}
	// Include all voucher types — Tally built-ins (Sales, Purchase, etc.) don't
	// populate <ISACTIVE> in collection responses, so ParseBool returns false for
	// them even though they are active. We include every type returned by Tally.
	out := make([]map[string]interface{}, 0, len(vtypes))
	for _, vt := range vtypes {
		// Tally doesn't export NatureOfVoucherType via NativeMethod in all versions.
		// Fall back to Parent which holds the base type name (Sales, Purchase, etc.)
		// and is equivalent to the voucher nature for DocType mapping purposes.
		nature := vt.NatureOfVoucherType
		if nature == "" {
			nature = vt.Parent
		}
		out = append(out, map[string]interface{}{
			"name":   vt.Name,
			"nature": nature,
		})
	}
	return out
}

// GetTallyStockItems returns stock items with UOM and HSN for data quality check.
func (a *App) GetTallyStockItems(company string) []map[string]interface{} {
	if a.tallyClient == nil {
		return nil
	}
	items, err := reader.ListStockItems(a.ctx, a.tallyClient, company, time.Time{}, time.Time{})
	if err != nil {
		fmt.Printf("[GetTallyStockItems] error: %v\n", err)
		return nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]interface{}{
			"name":    it.Name,
			"parent":  it.Parent,
			"baseUOM": it.BaseUnits,
			"hsn":     it.HSNCode,
		})
	}
	return out
}

// ─── ERPNext data accessors ──────────────────────────────────────────────────

// GetERPNextCompanies fetches company names from ERPNext.
func (a *App) GetERPNextCompanies() []string {
	data, status, err := a.erpCall("GET", `/api/resource/Company?fields=["name"]&limit=50`, nil)
	if err != nil || status != 200 {
		return nil
	}
	var resp struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil
	}
	out := make([]string, len(resp.Data))
	for i, c := range resp.Data {
		out[i] = c.Name
	}
	return out
}

// GetERPNextFiscalYears fetches fiscal years from ERPNext.
func (a *App) GetERPNextFiscalYears() []map[string]string {
	data, status, err := a.erpCall("GET", `/api/resource/Fiscal Year?fields=["name","year_start_date","year_end_date"]&limit=20`, nil)
	if err != nil || status != 200 {
		return nil
	}
	var resp struct {
		Data []struct {
			Name          string `json:"name"`
			YearStartDate string `json:"year_start_date"`
			YearEndDate   string `json:"year_end_date"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil
	}
	out := make([]map[string]string, len(resp.Data))
	for i, fy := range resp.Data {
		out[i] = map[string]string{
			"name":      fy.Name,
			"yearStart": fy.YearStartDate,
			"yearEnd":   fy.YearEndDate,
		}
	}
	return out
}

// GetERPNextAccounts fetches accounts for the given ERPNext company.
// Returns both leaf and group accounts so the user can map Tally ledger groups
// to any ERPNext account (including parent/control accounts like "Debtors").
func (a *App) GetERPNextAccounts(company string) []map[string]string {
	filters := fmt.Sprintf(`[["company","=","%s"]]`, company)
	fields := `["name","account_type","root_type","parent_account","is_group"]`
	path := "/api/resource/Account?" + url.Values{
		"filters":  {filters},
		"fields":   {fields},
		"limit":    {"1000"},
		"order_by": {"name asc"},
	}.Encode()
	data, status, err := a.erpCall("GET", path, nil)
	if err != nil {
		fmt.Printf("[GetERPNextAccounts] HTTP error for company %q: %v\n", company, err)
		return nil
	}
	if status != 200 {
		fmt.Printf("[GetERPNextAccounts] HTTP %d for company %q: %s\n", status, company, string(data))
		return nil
	}
	var resp struct {
		Data []struct {
			Name          string      `json:"name"`
			AccountType   string      `json:"account_type"`
			RootType      string      `json:"root_type"`
			ParentAccount string      `json:"parent_account"`
			IsGroup       interface{} `json:"is_group"` // 0 or 1 in Frappe
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Printf("[GetERPNextAccounts] unmarshal error: %v\n", err)
		return nil
	}
	out := make([]map[string]string, 0, len(resp.Data))
	for _, a := range resp.Data {
		isGroup := "0"
		switch v := a.IsGroup.(type) {
		case float64:
			if v == 1 {
				isGroup = "1"
			}
		case string:
			if v == "1" || v == "Yes" {
				isGroup = "1"
			}
		}
		out = append(out, map[string]string{
			"name":          a.Name,
			"accountType":   a.AccountType,
			"rootType":      a.RootType,
			"parentAccount": a.ParentAccount,
			"isGroup":       isGroup,
		})
	}
	return out
}

// ─── Setup validation ─────────────────────────────────────────────────────────

// ValidateSetup checks that ERPNext has the company, fiscal year, and required
// apps in place before activation. Returns a list of check results.
func (a *App) ValidateSetup(erpCompany, fromDate, toDate string) []map[string]interface{} {
	checks := []map[string]interface{}{}

	add := func(label, status, message string, canFix bool) {
		checks = append(checks, map[string]interface{}{
			"label":   label,
			"status":  status,
			"message": message,
			"canFix":  canFix,
		})
	}

	// 1. ERPNext company exists
	data, status, err := a.erpCall("GET", fmt.Sprintf(`/api/resource/Company/%s`, erpCompany), nil)
	switch {
	case err != nil:
		add("ERPNext Company", "error", "Cannot reach ERPNext: "+err.Error(), false)
	case status == 404 || (status != 200 && status != 302):
		add("ERPNext Company", "fail", fmt.Sprintf("Company %q not found in ERPNext", erpCompany), true)
	default:
		_ = data
		add("ERPNext Company", "ok", erpCompany+" exists", false)
	}

	// 2. Fiscal year covering the sync date range
	fyData, fyStatus, _ := a.erpCall("GET", fmt.Sprintf(
		`/api/resource/Fiscal Year?filters=[["year_start_date","<=","%s"],["year_end_date",">=","%s"]]&fields=["name"]&limit=1`,
		fromDate, toDate,
	), nil)
	if fyStatus == 200 {
		var fyResp struct {
			Data []struct{ Name string `json:"name"` } `json:"data"`
		}
		if json.Unmarshal(fyData, &fyResp) == nil && len(fyResp.Data) > 0 {
			add("Fiscal Year", "ok", fyResp.Data[0].Name, false)
		} else {
			add("Fiscal Year", "fail", fmt.Sprintf("No fiscal year covering %s → %s", fromDate, toDate), true)
		}
	} else {
		add("Fiscal Year", "error", "Could not query fiscal years", false)
	}

	// 3. India Compliance — check the DocType itself exists (works for Single doctypes too).
	// GET /api/resource/DocType/GST Settings returns 200 if the doctype is registered,
	// 403 if it exists but access is restricted, 404 if the app is not installed.
	_, icStatus, _ := a.erpCall("GET", `/api/resource/DocType/GST Settings`, nil)
	if icStatus == 200 || icStatus == 403 {
		add("India Compliance App", "ok", "India Compliance installed (GST Settings doctype found)", false)
	} else {
		// Also try the single-document endpoint as fallback
		_, icStatus2, _ := a.erpCall("GET", `/api/resource/GST Settings/GST Settings`, nil)
		if icStatus2 == 200 || icStatus2 == 403 {
			add("India Compliance App", "ok", "India Compliance installed (GST Settings accessible)", false)
		} else {
			add("India Compliance App", "fail",
				"India Compliance not detected. Install via: bench get-app india_compliance && bench --site {site} install-app india_compliance",
				false)
		}
	}

	// 4. Chart of accounts — check via the Company document's chart_of_accounts field.
	// This avoids Account list permission issues that vary by user role.
	coaData, coaStatus, _ := a.erpCall("GET", fmt.Sprintf(`/api/resource/Company/%s?fields=["chart_of_accounts","abbr"]`, erpCompany), nil)
	if coaStatus == 200 {
		var coaResp struct {
			Data struct {
				ChartOfAccounts string `json:"chart_of_accounts"`
				Abbr            string `json:"abbr"`
			} `json:"data"`
		}
		if json.Unmarshal(coaData, &coaResp) == nil && coaResp.Data.ChartOfAccounts != "" {
			add("Chart of Accounts", "ok",
				fmt.Sprintf("CoA: %s (abbr: %s)", coaResp.Data.ChartOfAccounts, coaResp.Data.Abbr), false)
		} else {
			add("Chart of Accounts", "warn", "Company has no chart_of_accounts set — accounts may not exist yet", true)
		}
	} else {
		add("Chart of Accounts", "warn", fmt.Sprintf("Could not read company details (HTTP %d) — check API permissions", coaStatus), false)
	}

	// 5. TallyBridge custom fields — optional, only warn if missing; provide install command.
	cfData, cfStatus, _ := a.erpCall("GET", `/api/resource/Custom Field?filters=[["dt","=","Sales Invoice"],["fieldname","=","tally_guid"]]&limit=1`, nil)
	if cfStatus == 200 {
		var cfResp struct {
			Data []struct{ Name string `json:"name"` } `json:"data"`
		}
		if json.Unmarshal(cfData, &cfResp) == nil && len(cfResp.Data) > 0 {
			add("TallyBridge Custom Fields", "ok", "tally_guid custom field found on Sales Invoice", false)
		} else {
			add("TallyBridge Custom Fields", "warn",
				"Not installed yet. Run: bench get-app tallybridge && bench --site {site} install-app tallybridge — or install manually via Customize Form.",
				false)
		}
	} else {
		add("TallyBridge Custom Fields", "warn",
			"Could not verify. Install manually: Customize Form → Sales Invoice → add field tally_guid (Data, read-only).",
			false)
	}

	return checks
}

// CreateERPNextFiscalYear creates a fiscal year in ERPNext.
func (a *App) CreateERPNextFiscalYear(yearName, fromDate, toDate, company string) map[string]interface{} {
	body := map[string]interface{}{
		"doctype":         "Fiscal Year",
		"year":            yearName,
		"year_start_date": fromDate,
		"year_end_date":   toDate,
		"companies": []map[string]string{
			{"company": company},
		},
	}
	data, status, err := a.erpCall("POST", "/api/resource/Fiscal Year", body)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": err.Error()}
	}
	if status != 200 && status != 201 {
		return map[string]interface{}{"status": "error", "message": fmt.Sprintf("ERPNext returned %d: %s", status, string(data))}
	}
	return map[string]interface{}{"status": "ok", "message": "Fiscal year " + yearName + " created"}
}

// SaveERPNextMapping stores the chosen ERPNext company name.
func (a *App) SaveERPNextMapping(erpCompany string) error {
	return a.db.SetConfig("erpnext_company", erpCompany)
}

// SyncNow triggers an immediate sync cycle from the UI.
// If fullSync is true, the watermark is reset so all vouchers within the
// configured FY date range are re-sent to the cloud (full historical import).
// If fullSync is false, only vouchers altered since the last sync are sent.
// PushMappingsToCloud sends the connector's locally-configured mapping sections
// and ERPNext credentials to the cloud via PUT /connector/config.
// Call this on every sync so the cloud always has up-to-date config.
func (a *App) PushMappingsToCloud() error {
	cloudURL := a.db.GetConfig("cloud_url")
	token := a.db.GetConfig("machine_token")
	companyID := a.db.GetConfig("company_id")
	if cloudURL == "" || token == "" || companyID == "" {
		return fmt.Errorf("not registered with cloud")
	}

	// Push ERPNext credentials + any locally-configured account mappings.
	// Cloud uses || (JSONB merge) so cloud UI edits always win — local entries
	// only add new keys, never overwrite existing ones.
	// GST and voucher type mappings are auto-seeded by autoFetchAndSeedCompany,
	// so we only push account_coa (user-configured specific ledger overrides).
	accountMappings := make(map[string]interface{})
	if raw := a.db.GetMappingConfig(companyID, "account_coa"); raw != "{}" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			for k, v := range parsed {
				if len(k) > 0 && k[0] == '_' {
					continue // strip meta keys like _type
				}
				if v == "" || v == nil {
					continue // strip empty values
				}
				accountMappings[k] = v
			}
		}
	}

	body := map[string]interface{}{
		"companyId":         companyID,
		"erpnextUrl":        a.db.GetConfig("erpnext_url"),
		"erpnextApiKey":     a.db.GetConfig("erpnext_api_key"),
		"erpnextApiSecret":  a.db.GetConfig("erpnext_api_secret"),
		"erpnextCompany":    a.db.GetConfig("erpnext_company"),
		"erpnextGstin":      a.db.GetConfig("erpnext_gstin"),
		"erpnextWarehouse":  a.db.GetConfig("erpnext_warehouse"),
		"erpnextCostCenter": a.db.GetConfig("erpnext_cost_center"),
		"accountMappings":   accountMappings,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, "PUT", cloudURL+"/connector/config", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PUT /connector/config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cloud returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (a *App) SyncNow(fullSync bool) map[string]interface{} {
	cloudURL := a.db.GetConfig("cloud_url")
	token := a.db.GetConfig("machine_token")
	companyID := a.db.GetConfig("company_id")
	companyName := a.db.GetConfig("company_tally_name")

	if cloudURL == "" || token == "" || companyID == "" || companyName == "" {
		return map[string]interface{}{"status": "error", "message": "Not registered with cloud — complete setup first"}
	}
	if a.tallyClient == nil {
		return map[string]interface{}{"status": "error", "message": "Tally client not initialised"}
	}

	// Push ERPNext credentials to cloud first — this triggers autoFetchAndSeedCompany
	// which seeds GST + voucher type defaults. Then pull the seeded config back.
	if err := a.PushMappingsToCloud(); err != nil {
		fmt.Printf("[SyncNow] Warning: could not push credentials to cloud: %v\n", err)
	} else {
		fmt.Println("[SyncNow] Credentials pushed to cloud")
	}
	if r := a.GetCloudConfig(); r["status"] != "ok" {
		fmt.Printf("[SyncNow] Warning: could not pull config from cloud: %v\n", r["message"])
	} else {
		fmt.Println("[SyncNow] Config pulled from cloud")
	}

	if fullSync {
		// Reset watermarks → DeltaSync will pick up all vouchers + masters from scratch
		if err := a.db.SetLastSyncAt("vouchers", ""); err != nil {
			fmt.Printf("[SyncNow] failed to reset voucher watermark: %v\n", err)
		}
		if err := a.db.SetLastSyncAt("masters", ""); err != nil {
			fmt.Printf("[SyncNow] failed to reset masters watermark: %v\n", err)
		}
		if err := a.db.ClearMasterHashes(); err != nil {
			fmt.Printf("[SyncNow] failed to clear master hashes: %v\n", err)
		}
		fmt.Println("[SyncNow] All watermarks + hashes reset — full sync mode")
	}

	if err := tbsync.DeltaSync(a.ctx, a.tallyClient, a.db, cloudURL, token, companyID, companyName); err != nil {
		return map[string]interface{}{"status": "error", "message": err.Error()}
	}

	lastSync := a.db.GetLastSyncAt("vouchers")
	msg := fmt.Sprintf("Sync completed. Watermark: %s", lastSync)
	if fullSync {
		msg = "Full sync completed. All vouchers re-sent. Watermark: " + lastSync
	}
	return map[string]interface{}{"status": "ok", "message": msg, "lastSyncAt": lastSync}
}

// ─── Service status and sync stats ───────────────────────────────────────────

// GetServiceStatus returns health status of Tally, ERPNext, and the cloud.
func (a *App) GetServiceStatus() map[string]interface{} {
	result := map[string]interface{}{}

	// Tally
	if a.tallyClient != nil {
		info := reader.Heartbeat(a.ctx, a.tallyClient)
		result["tally"] = map[string]interface{}{
			"online":  info.Online,
			"company": info.OpenCompany,
		}
	} else {
		result["tally"] = map[string]interface{}{"online": false}
	}

	// ERPNext
	erpURL := a.db.GetConfig("erpnext_url")
	if erpURL != "" {
		_, status, err := a.erpCall("GET", "/api/method/frappe.auth.get_logged_user", nil)
		result["erpnext"] = map[string]interface{}{
			"online": err == nil && status == 200,
			"url":    erpURL,
		}
	} else {
		result["erpnext"] = map[string]interface{}{"online": false, "url": ""}
	}

	// Cloud
	cloudURL := a.db.GetConfig("cloud_url")
	if cloudURL != "" {
		cl := &http.Client{Timeout: 5 * time.Second}
		resp, err := cl.Get(cloudURL + "/health")
		cloudOnline := err == nil && resp != nil && resp.StatusCode == 200
		if resp != nil {
			resp.Body.Close()
		}
		result["cloud"] = map[string]interface{}{
			"online": cloudOnline,
			"url":    cloudURL,
		}
	} else {
		result["cloud"] = map[string]interface{}{"online": false, "url": ""}
	}

	return result
}

// GetSyncStats returns last sync watermark and counts from the local DB.
func (a *App) GetSyncStats() map[string]interface{} {
	lastSync := a.db.GetLastSyncAt("vouchers")
	totalSynced := a.db.GetConfig("total_synced_count")
	if totalSynced == "" {
		totalSynced = "0"
	}
	return map[string]interface{}{
		"lastSyncAt":  lastSync,
		"totalSynced": totalSynced,
		"companyId":   a.db.GetConfig("company_id"),
		"companyName": a.db.GetConfig("company_tally_name"),
	}
}

// GetAllConfig returns all key-value config for the Settings tab.
func (a *App) GetAllConfig() map[string]string {
	keys := []string{
		"erpnext_url", "erpnext_api_key", "erpnext_api_secret",
		"erpnext_company", "company_tally_name", "cloud_url",
		"sync_from_date", "sync_to_date", "machine_id", "company_id",
		"last_registered_at", "poll_interval_minutes",
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = a.db.GetConfig(k)
	}
	return out
}

// SetConfigValue updates a single config key from the Settings tab.
func (a *App) SetConfigValue(key, value string) error {
	return a.db.SetConfig(key, value)
}

// GetMappingDefaults returns seed/default JSON for a mapping section.
// Useful for first-time setup — the UI shows a "Load Defaults" button that
// calls this and pre-fills the JSON editor.
func (a *App) GetMappingDefaults(section string) string {
	switch section {
	case "account_coa":
		return `{"_type":"group","Sundry Debtors":"","Sundry Creditors":"","Bank Accounts":"","Cash-in-Hand":"","Duties & Taxes":"","Sales Accounts":"","Purchase Accounts":"","Direct Expenses":"","Indirect Expenses":"","Direct Incomes":"","Indirect Incomes":"","Fixed Assets":"","Capital Account":"","Reserves & Surplus":"","Investments":"","Loans & Advances (Asset)":"","Loans (Liability)":""}`
	case "ledger_group":
		return `{"Sundry Debtors":"Receivable","Sundry Creditors":"Payable","Bank Accounts":"Bank","Cash-in-Hand":"Cash","Duties & Taxes":"Tax","Sales Accounts":"Income","Purchase Accounts":"Expense","Direct Expenses":"Expense","Indirect Expenses":"Expense","Direct Incomes":"Income","Indirect Incomes":"Income","Fixed Assets":"Fixed Asset","Capital Account":"Equity","Reserves & Surplus":"Equity","Investments":"Asset","Loans & Advances (Asset)":"Asset","Loans (Liability)":"Liability"}`
	case "gst_ledger":
		return `{"Output IGST":{"account":"","gstType":"IGST","direction":"output"},"Output CGST":{"account":"","gstType":"CGST","direction":"output"},"Output SGST":{"account":"","gstType":"SGST","direction":"output"},"Input IGST":{"account":"","gstType":"IGST","direction":"input"},"Input CGST":{"account":"","gstType":"CGST","direction":"input"},"Input SGST":{"account":"","gstType":"SGST","direction":"input"}}`
	case "item_hsn":
		return `{"_strategy":"auto","_note":"auto: sync Tally HSNCode field directly. manual: add item overrides below.","overrides":{}}`
	case "uom":
		return `{"Nos":"Nos","Kgs":"Kg","Ltrs":"Litre","Mtr":"Metre","Pcs":"Pcs","Box":"Box","Bag":"Bag","Set":"Set","Pair":"Pair","Dozen":"Dozen","Quintal":"Quintal","MT":"MT","Gms":"Gram","Ml":"Millilitre","Cm":"Centimetre","Feet":"Feet","Inch":"Inch","Sq.Ft":"Square Feet","Sq.Mtr":"Square Metre"}`
	case "party_identity":
		return `{"deduplicateBy":["gstin","pan"],"createOnFirstSync":true,"updateExisting":false,"skipIfGSTINMissing":false,"defaultCustomerGroup":"All Customer Groups","defaultSupplierGroup":"All Supplier Groups","defaultTerritoryCustomer":"India","defaultTerritorySupplier":""}`
	case "sync_direction":
		return `{"tallyToERP":true,"erpToTally":false,"salesInvoiceSeries":"SINV-","purchaseInvoiceSeries":"PINV-","paymentEntrySeries":"PAY-","journalEntrySeries":"JV-","skipFutureVouchers":true,"skipCancelledVouchers":true}`
	case "conflict_resolution":
		return `{"strategy":"tally_wins","onDuplicate":"skip","onMissingAccount":"exception","onMissingParty":"create","onMissingItem":"create","retryFailedAfterMinutes":30}`
	case "historical_import":
		return `{"enabled":true,"batchSize":100,"skipAlreadySynced":true,"onlyVoucherTypes":[],"skipVoucherTypes":["Attendance"],"startFromDate":"","endAtDate":""}`
	case "exception_handling":
		return `{"maxRetries":3,"retryDelayMinutes":15,"notifyOnException":false,"exceptionWebhookUrl":"","skipAfterMaxRetries":true,"logLevel":"warn"}`
	}
	return "{}"
}

// FetchAPITokenViaLogin logs into ERPNext with username+password, generates an API key
// for that user (if not already generated), and returns the key+secret.
func (a *App) FetchAPITokenViaLogin(erpUrl, username, password string) map[string]interface{} {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 15 * time.Second, Jar: jar}

	// Step 1: login
	loginURL := strings.TrimRight(erpUrl, "/") + "/api/method/login"
	form := url.Values{"usr": {username}, "pwd": {password}}
	resp, err := client.PostForm(loginURL, form)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": "Login request failed: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return map[string]interface{}{"status": "error", "message": fmt.Sprintf("Login failed (HTTP %d): %s", resp.StatusCode, string(body))}
	}

	// Step 2: generate (or retrieve) API key for this user
	genURL := strings.TrimRight(erpUrl, "/") + "/api/method/frappe.core.doctype.user.user.generate_keys?user=" + url.QueryEscape(username)
	req2, _ := http.NewRequest("POST", genURL, nil)
	req2.Header.Set("X-Frappe-CSRF-Token", "fetch") // Frappe accepts "fetch" as CSRF bypass for same-origin
	resp2, err := client.Do(req2)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": "Generate keys request failed: " + err.Error()}
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode != 200 {
		return map[string]interface{}{"status": "error", "message": fmt.Sprintf("Generate keys failed (HTTP %d): %s", resp2.StatusCode, string(body2))}
	}

	// Parse {"message": {"api_secret": "...", "api_key": "..."}}
	var parsed struct {
		Message struct {
			APIKey    string `json:"api_key"`
			APISecret string `json:"api_secret"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body2, &parsed); err != nil || parsed.Message.APIKey == "" {
		// api_key may already exist and not be returned — fetch it from User doc
		docURL := strings.TrimRight(erpUrl, "/") + "/api/resource/User/" + url.QueryEscape(username)
		req3, _ := http.NewRequest("GET", docURL, nil)
		resp3, err := client.Do(req3)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": "Could not fetch user doc: " + err.Error()}
		}
		defer resp3.Body.Close()
		body3, _ := io.ReadAll(resp3.Body)
		var doc struct {
			Data struct {
				APIKey    string `json:"api_key"`
				APISecret string `json:"api_secret"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body3, &doc); err != nil || doc.Data.APIKey == "" {
			return map[string]interface{}{"status": "error", "message": "Could not read API key from User doc. Try manual entry."}
		}
		return map[string]interface{}{"status": "ok", "apiKey": doc.Data.APIKey, "apiSecret": doc.Data.APISecret}
	}

	return map[string]interface{}{"status": "ok", "apiKey": parsed.Message.APIKey, "apiSecret": parsed.Message.APISecret}
}

// CreateERPNextCustomFields creates the TallyBridge custom fields on ERPNext doctypes.
// Idempotent — skips fields that already exist.
func (a *App) CreateERPNextCustomFields() map[string]interface{} {
	type fieldDef struct {
		DT         string `json:"dt"`
		Fieldname  string `json:"fieldname"`
		Label      string `json:"label"`
		Fieldtype  string `json:"fieldtype"`
		InsertAfter string `json:"insert_after"`
		ReadOnly   int    `json:"read_only"`
		NoCopy     int    `json:"no_copy"`
		Unique     int    `json:"unique,omitempty"`
		InFilter   int    `json:"in_filter,omitempty"`
		SearchIndex int   `json:"search_index,omitempty"`
	}

	allDTs := []string{"Sales Invoice", "Purchase Invoice", "Journal Entry", "Payment Entry", "Item", "Customer", "Supplier"}
	txnDTs := []string{"Sales Invoice", "Purchase Invoice", "Journal Entry", "Payment Entry"}

	var fields []fieldDef
	for _, dt := range allDTs {
		fields = append(fields, fieldDef{DT: dt, Fieldname: "custom_tally_guid", Label: "Tally GUID", Fieldtype: "Data", InsertAfter: "naming_series", ReadOnly: 1, NoCopy: 1, Unique: 1, InFilter: 1, SearchIndex: 1})
		fields = append(fields, fieldDef{DT: dt, Fieldname: "custom_sync_source", Label: "Sync Source", Fieldtype: "Data", InsertAfter: "custom_tally_guid", ReadOnly: 1, NoCopy: 1})
	}
	for _, dt := range txnDTs {
		fields = append(fields, fieldDef{DT: dt, Fieldname: "custom_tally_voucher_number", Label: "Tally Voucher Number", Fieldtype: "Data", InsertAfter: "custom_sync_source", ReadOnly: 1, NoCopy: 1})
	}

	created, skipped, errCount := 0, 0, 0
	var errMsgs []string

	for _, f := range fields {
		body, _ := json.Marshal(f)
		data, status, err := a.erpCallRaw("POST", "/api/resource/Custom%20Field", body)
		if err != nil {
			errCount++
			errMsgs = append(errMsgs, f.DT+"."+f.Fieldname+": "+err.Error())
			continue
		}
		resp := string(data)
		switch {
		case status == 200 && strings.Contains(resp, `"name"`):
			created++
		case strings.Contains(resp, "DuplicateEntryError") || strings.Contains(resp, "already exists"):
			skipped++
		default:
			errCount++
			if len(resp) > 120 {
				resp = resp[:120]
			}
			errMsgs = append(errMsgs, f.DT+"."+f.Fieldname+": "+resp)
		}
	}

	return map[string]interface{}{
		"status":  "ok",
		"created": created,
		"skipped": skipped,
		"errors":  errCount,
		"details": strings.Join(errMsgs, "\n"),
	}
}

// erpCallRaw is like erpCall but accepts a raw []byte body (already marshalled).
func (a *App) erpCallRaw(method, path string, body []byte) ([]byte, int, error) {
	erpURL := a.db.GetConfig("erpnext_url")
	apiKey := a.db.GetConfig("erpnext_api_key")
	apiSecret := a.db.GetConfig("erpnext_api_secret")
	if erpURL == "" {
		return nil, 0, fmt.Errorf("ERPNext URL not configured")
	}
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(a.ctx, method, erpURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("token %s:%s", apiKey, apiSecret))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	cl := &http.Client{Timeout: 15 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// cloudCall makes an authenticated HTTP call to the cloud platform.
func (a *App) cloudCall(method, path string, body interface{}) ([]byte, int, error) {
	cloudURL := a.db.GetConfig("cloud_url")
	token := a.db.GetConfig("machine_token")
	if cloudURL == "" {
		return nil, 0, fmt.Errorf("cloud URL not configured")
	}
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(a.ctx, method, cloudURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// GetSyncExceptions fetches recent sync exceptions from the cloud platform.
func (a *App) GetSyncExceptions(limit int) []map[string]interface{} {
	companyID := a.db.GetConfig("company_id")
	if companyID == "" {
		return []map[string]interface{}{}
	}
	if limit <= 0 {
		limit = 100
	}
	path := fmt.Sprintf("/connector/exceptions?limit=%d", limit)
	data, status, err := a.cloudCall("GET", path, nil)
	if err != nil || status != 200 {
		return []map[string]interface{}{}
	}
	var resp struct {
		Exceptions []map[string]interface{} `json:"exceptions"`
	}
	if json.Unmarshal(data, &resp) != nil || resp.Exceptions == nil {
		return []map[string]interface{}{}
	}
	return resp.Exceptions
}

// GetTallyCounts queries Tally for voucher and master counts and returns them.
// Called periodically to update the cloud's tally_counts record.
func (a *App) GetTallyCounts() map[string]interface{} {
	tallyURL := a.db.GetConfig("tally_url")
	companyName := a.db.GetConfig("company_tally_name")
	fromDate := a.db.GetConfig("sync_from_date")
	toDate := a.db.GetConfig("sync_to_date")

	if tallyURL == "" || companyName == "" {
		return map[string]interface{}{"status": "error", "message": "Tally not configured"}
	}

	counts := make(map[string]interface{})

	// Get voucher counts
	voucherCounts, err := reader.GetVoucherCounts(tallyURL, companyName, fromDate, toDate)
	if err == nil {
		for k, v := range voucherCounts {
			counts[k] = v
		}
	}

	// Get master counts
	masterCounts, err := reader.GetMasterCounts(tallyURL, companyName)
	if err == nil {
		for k, v := range masterCounts {
			counts[k] = v
		}
	}

	return map[string]interface{}{"status": "ok", "counts": counts}
}

// ReprocessException triggers reprocessing of a specific exception (or all if id == "all").
func (a *App) ReprocessException(exceptionId string) map[string]interface{} {
	companyID := a.db.GetConfig("company_id")
	if companyID == "" {
		return map[string]interface{}{"status": "error", "message": "Not registered with cloud"}
	}
	body := map[string]interface{}{"companyId": companyID}
	if exceptionId != "" && exceptionId != "all" {
		body["exceptionIds"] = []string{exceptionId}
	}
	data, status, err := a.cloudCall("POST", "/connector/reprocess-exceptions", body)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": err.Error()}
	}
	if status != 200 {
		return map[string]interface{}{"status": "error", "message": fmt.Sprintf("Cloud returned %d: %s", status, string(data))}
	}
	return map[string]interface{}{"status": "ok", "message": "Reprocessing triggered"}
}

// GetSyncHistory fetches recent sync records from the cloud platform.
func (a *App) GetSyncHistory(limit int) []map[string]interface{} {
	companyID := a.db.GetConfig("company_id")
	if companyID == "" {
		return []map[string]interface{}{}
	}
	if limit <= 0 {
		limit = 200
	}
	path := fmt.Sprintf("/connector/sync-records?limit=%d", limit)
	data, status, err := a.cloudCall("GET", path, nil)
	if err != nil || status != 200 {
		return []map[string]interface{}{}
	}
	var resp struct {
		Records []map[string]interface{} `json:"records"`
	}
	if json.Unmarshal(data, &resp) != nil || resp.Records == nil {
		return []map[string]interface{}{}
	}
	return resp.Records
}

// CheckForUpdate checks the GitHub releases API for a newer version of the connector.
func (a *App) CheckForUpdate() map[string]interface{} {
	req, err := http.NewRequestWithContext(a.ctx, "GET",
		"https://api.github.com/repos/OneBook-ERP/tallybridge-connector/releases/latest", nil)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": err.Error()}
	}
	req.Header.Set("User-Agent", "TallyBridge-Connector")
	cl := &http.Client{Timeout: 8 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return map[string]interface{}{"status": "error", "message": "Network error: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return map[string]interface{}{"status": "error", "message": fmt.Sprintf("GitHub returned %d", resp.StatusCode)}
	}
	var rel struct {
		TagName    string `json:"tag_name"`
		Name       string `json:"name"`
		HTMLURL    string `json:"html_url"`
		PreRelease bool   `json:"prerelease"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &rel); err != nil {
		return map[string]interface{}{"status": "error", "message": "Parse error: " + err.Error()}
	}
	latestVersion := strings.TrimPrefix(rel.TagName, "v")
	currentVersion := strings.TrimPrefix(Version, "v")
	hasUpdate := latestVersion != "" && latestVersion != currentVersion
	return map[string]interface{}{
		"status":         "ok",
		"currentVersion": currentVersion,
		"latestVersion":  latestVersion,
		"hasUpdate":      hasUpdate,
		"releaseName":    rel.Name,
		"releaseUrl":     rel.HTMLURL,
		"prerelease":     rel.PreRelease,
	}
}

// GetDiagnostics measures connectivity and latency to each service.
func (a *App) GetDiagnostics() map[string]interface{} {
	result := map[string]interface{}{}

	// Tally
	if a.tallyClient != nil {
		start := time.Now()
		info := reader.Heartbeat(a.ctx, a.tallyClient)
		latency := time.Since(start).Milliseconds()
		result["tally"] = map[string]interface{}{
			"online":  info.Online,
			"latency": latency,
			"company": info.OpenCompany,
		}
	} else {
		result["tally"] = map[string]interface{}{"online": false, "latency": int64(-1)}
	}

	// ERPNext
	erpURL := a.db.GetConfig("erpnext_url")
	if erpURL != "" {
		start := time.Now()
		_, status, err := a.erpCall("GET", "/api/method/frappe.auth.get_logged_user", nil)
		latency := time.Since(start).Milliseconds()
		result["erpnext"] = map[string]interface{}{
			"online":  err == nil && status == 200,
			"latency": latency,
			"url":     erpURL,
		}
	} else {
		result["erpnext"] = map[string]interface{}{"online": false, "latency": int64(-1), "url": ""}
	}

	// Cloud
	cloudURL := a.db.GetConfig("cloud_url")
	if cloudURL != "" {
		start := time.Now()
		cl := &http.Client{Timeout: 5 * time.Second}
		resp, err := cl.Get(cloudURL + "/health")
		latency := time.Since(start).Milliseconds()
		cloudOnline := err == nil && resp != nil && resp.StatusCode == 200
		if resp != nil {
			resp.Body.Close()
		}
		result["cloud"] = map[string]interface{}{
			"online":  cloudOnline,
			"latency": latency,
			"url":     cloudURL,
		}
	} else {
		result["cloud"] = map[string]interface{}{"online": false, "latency": int64(-1), "url": ""}
	}

	return result
}

// SyncMasters clears the masters watermark and immediately runs a full master sync
// (ledgers, stock items, groups) without syncing vouchers. Use this before the first
// voucher sync so Customers, Suppliers, Items and their groups exist in ERPNext.
func (a *App) SyncMasters() map[string]interface{} {
	companyID := a.db.GetConfig("company_id")
	cloudURL := a.db.GetConfig("cloud_url")
	token := a.db.GetConfig("machine_token")
	companyName := a.db.GetConfig("company_tally_name")
	if companyID == "" || cloudURL == "" || token == "" {
		return map[string]interface{}{"status": "error", "message": "Not registered with cloud"}
	}
	// Clear the masters watermark so DeltaSync fetches all master data fresh
	if err := a.db.SetLastSyncAt("masters", ""); err != nil {
		return map[string]interface{}{"status": "error", "message": "Failed to clear masters watermark: " + err.Error()}
	}
	// Delete all master hashes so filterChanged* treats all records as new
	if err := a.db.ClearMasterHashes(); err != nil {
		return map[string]interface{}{"status": "error", "message": "Failed to clear master hashes: " + err.Error()}
	}
	if err := tbsync.DeltaSync(a.ctx, a.tallyClient, a.db, cloudURL, token, companyID, companyName, true); err != nil {
		return map[string]interface{}{"status": "error", "message": "Master sync failed: " + err.Error()}
	}
	return map[string]interface{}{"status": "ok", "message": "Masters synced — check Exceptions tab before syncing transactions"}
}

// GetSyncEnabled returns whether background sync is enabled (default: true).
func (a *App) GetSyncEnabled() bool {
	v := a.db.GetConfig("sync_enabled")
	return v != "false"
}

// SetSyncEnabled enables or disables the background sync scheduler.
func (a *App) SetSyncEnabled(enabled bool) {
	v := "true"
	if !enabled {
		v = "false"
	}
	_ = a.db.SetConfig("sync_enabled", v)
}

// DeleteCompanyData wipes all local connector state for a clean re-migration:
// sync watermarks, master hashes, and all configuration keys.
// The cloud-side company record is NOT deleted — use the cloud dashboard for that.
func (a *App) DeleteCompanyData() map[string]interface{} {
	// Stop the scheduler if running
	if a.scheduler != nil {
		a.scheduler.Stop()
		a.scheduler = nil
	}
	// Clear sync watermarks
	for _, collection := range []string{"vouchers", "ledgers", "stock_items", "masters"} {
		_ = a.db.SetLastSyncAt(collection, "")
	}
	// Clear master hashes
	_ = a.db.ClearMasterHashes()
	// Clear all config keys
	configKeys := []string{
		"company_tally_name", "machine_token", "cloud_url", "company_id", "machine_id",
		"sync_from_date", "sync_to_date", "erpnext_url", "erpnext_api_key", "erpnext_api_secret",
		"erpnext_company", "tally_url", "tally_company", "erpnext_gstin",
		"erpnext_warehouse", "erpnext_cost_center", "sync_enabled", "last_registered_at",
	}
	for _, k := range configKeys {
		_ = a.db.SetConfig(k, "")
	}
	return map[string]interface{}{"status": "ok", "message": "Local data cleared"}
}

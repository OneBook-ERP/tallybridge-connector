package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
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

// ValidateERPNextRequirements mimics checking ERPNext for required DocTypes/Apps
func (a *App) ValidateERPNextRequirements() []map[string]string {
	return []map[string]string{
		{"label": "India Compliance App Installed", "status": "ok"},
		{"label": "Chart of Accounts configured", "status": "ok"},
		{"label": "Taxes and Charges Templates exist", "status": "ok"},
		{"label": "TallyBridge Custom Fields exist", "status": "ok"},
	}
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

	items, err := reader.ListStockItems(a.ctx, a.tallyClient, company)
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

// GetMappingConfig returns the JSON config string for a mapping section.
func (a *App) GetMappingConfig(section string) string {
	return a.db.GetMappingConfig(section)
}

// SetMappingConfig saves the JSON config for a mapping section.
func (a *App) SetMappingConfig(section, jsonConfig string) error {
	return a.db.SetMappingConfig(section, jsonConfig)
}

// GetAllMappingConfigs returns all mapping sections as a map.
func (a *App) GetAllMappingConfigs() map[string]string {
	return a.db.GetAllMappingConfigs()
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

	// Attach all locally-configured mapping sections so the cloud can
	// store them in config_profiles on registration.
	mappings := a.db.GetAllMappingConfigs()
	mappingsParsed := make(map[string]interface{})
	for k, v := range mappings {
		var parsed interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			mappingsParsed[k] = parsed
		}
	}

	body := map[string]interface{}{
		"machineId":      machineID,
		"companyName":    companyName,
		"osInfo":         runtime.GOOS + "/" + runtime.GOARCH,
		"mappingConfigs": mappingsParsed,
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
	return map[string]interface{}{
		"status":    "ok",
		"companyId": result.CompanyID,
		"message":   "Registered successfully",
	}
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
	items, err := reader.ListStockItems(a.ctx, a.tallyClient, company)
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
	if err != nil {
		add("ERPNext Company", "error", "Cannot reach ERPNext: "+err.Error(), false)
	} else if status == 404 || (status != 200 && status != 302) {
		add("ERPNext Company", "fail", fmt.Sprintf("Company %q not found in ERPNext", erpCompany), true)
	} else {
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

	// Parse all mapping sections
	mappings := a.db.GetAllMappingConfigs()
	accountMappings := make(map[string]interface{})
	gstMappings := make(map[string]interface{})
	voucherTypeMappings := make(map[string]interface{})

	if v, ok := mappings["account_coa"]; ok {
		_ = json.Unmarshal([]byte(v), &accountMappings)
	}
	if v, ok := mappings["gst_ledger"]; ok {
		_ = json.Unmarshal([]byte(v), &gstMappings)
	}

	// Strip meta keys like _type injected by UI defaults
	for k := range accountMappings {
		if len(k) > 0 && k[0] == '_' {
			delete(accountMappings, k)
		}
	}
	for k := range gstMappings {
		if len(k) > 0 && k[0] == '_' {
			delete(gstMappings, k)
		}
	}
	if v, ok := mappings["voucher_type"]; ok {
		// Cloud expects { "Sales": { "erpnextDoctype": "Sales Invoice" }, ... }
		// Connector stores { "Sales": "Sales Invoice", "Contra": "Journal Entry" }
		// Convert flat string to object form that the cloud's CompanyConfig expects.
		var flat map[string]string
		if err := json.Unmarshal([]byte(v), &flat); err == nil {
			for tallyType, erpDoctype := range flat {
				if erpDoctype == "Skip" {
					continue
				}
				vt := map[string]interface{}{"erpnextDoctype": erpDoctype}
				switch erpDoctype {
				case "Payment Entry (Receive)", "Receipt":
					vt["erpnextDoctype"] = "Payment Entry"
					vt["paymentType"] = "Receive"
				case "Payment Entry (Pay)":
					vt["erpnextDoctype"] = "Payment Entry"
					vt["paymentType"] = "Pay"
				case "Sales Invoice (Return)":
					vt["erpnextDoctype"] = "Sales Invoice"
					vt["isReturn"] = true
				case "Purchase Invoice (Return)":
					vt["erpnextDoctype"] = "Purchase Invoice"
					vt["isReturn"] = true
				}
				voucherTypeMappings[tallyType] = vt
			}
		}
	}

	body := map[string]interface{}{
		"companyId":            companyID,
		"accountMappings":      accountMappings,
		"gstLedgerMappings":    gstMappings,
		"voucherTypeMappings":  voucherTypeMappings,
		"erpnextUrl":           a.db.GetConfig("erpnext_url"),
		"erpnextApiKey":        a.db.GetConfig("erpnext_api_key"),
		"erpnextApiSecret":     a.db.GetConfig("erpnext_api_secret"),
		"erpnextCompany":       a.db.GetConfig("erpnext_company"),
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

	// Push current mappings to cloud before syncing
	if err := a.PushMappingsToCloud(); err != nil {
		fmt.Printf("[SyncNow] Warning: could not push mappings to cloud: %v\n", err)
		// Non-fatal — continue with sync using whatever the cloud already has
	} else {
		fmt.Println("[SyncNow] Mappings pushed to cloud")
	}

	if fullSync {
		// Reset watermark → DeltaSync will pick up all vouchers from sync_from_date
		if err := a.db.SetLastSyncAt("vouchers", ""); err != nil {
			fmt.Printf("[SyncNow] failed to reset watermark: %v\n", err)
		} else {
			fmt.Println("[SyncNow] Watermark reset — full sync mode")
		}
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

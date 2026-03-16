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
	"github.com/tallybridge/connector/tally/parser"
	"github.com/tallybridge/connector/tally/writer"
)

// TallyVoucherInput matches the JSON job payload produced by the platform's
// erp-to-tally transforms. Field names match the TypeScript interface exactly.
type TallyVoucherInput struct {
	Action           string                    `json:"action"`
	GUID             string                    `json:"guid"`
	VoucherType      string                    `json:"voucherType"`
	ObjView          string                    `json:"objView"`
	Date             string                    `json:"date"` // yyyymmdd
	VoucherNumber    string                    `json:"voucherNumber"`
	ReferenceNumber  string                    `json:"referenceNumber"`
	ReferenceDate    string                    `json:"referenceDate"` // yyyymmdd
	PartyLedgerName  string                    `json:"partyLedgerName"`
	Narration        string                    `json:"narration"`
	IsInvoice        bool                      `json:"isInvoice"`
	PlaceOfSupply    string                    `json:"placeOfSupply"`
	IsRCMApplicable  bool                      `json:"isRCMApplicable"`
	LedgerEntries    []TallyLedgerEntryInput   `json:"ledgerEntries"`
	InventoryEntries []TallyInventoryEntryInput `json:"inventoryEntries"`
}

type TallyLedgerEntryInput struct {
	LedgerName       string                    `json:"ledgerName"`
	IsDeemedPositive bool                      `json:"isDeemedPositive"`
	IsPartyLedger    bool                      `json:"isPartyLedger"`
	Amount           float64                   `json:"amount"`
	BillAllocations  []TallyBillAllocationInput `json:"billAllocations"`
}

type TallyBillAllocationInput struct {
	Name     string  `json:"name"`
	BillType string  `json:"billType"`
	Amount   float64 `json:"amount"`
	DueDate  string  `json:"dueDate"`
}

type TallyInventoryEntryInput struct {
	StockItemName string  `json:"stockItemName"`
	Rate          float64 `json:"rate"`
	RateUOM       string  `json:"rateUOM"`
	Amount        float64 `json:"amount"`
	BilledQty     float64 `json:"billedQty"`
	BilledQtyUOM  string  `json:"billedQtyUOM"`
	HSNCode       string  `json:"hsnCode"`
	AccountLedger string  `json:"accountLedger"`
}

// pendingJobsResponse is the shape of GET /connector/jobs/pending.
type pendingJobsResponse struct {
	Jobs []pendingJob `json:"jobs"`
}

type pendingJob struct {
	JobID       string                 `json:"jobId"`
	Action      string                 `json:"action"`
	VoucherType string                 `json:"voucherType"`
	Payload     map[string]interface{} `json:"payload"`
}

// jobResultBody is the body for POST /connector/sync-result.
type jobResultBody struct {
	JobID         string `json:"jobId"`
	CompanyID     string `json:"companyId"`
	Status        string `json:"status"` // "SUCCESS" | "ERROR"
	TallyGUID     string `json:"tallyGuid,omitempty"`
	ErrorMessage  string `json:"errorMessage,omitempty"`
	TallyResponse string `json:"tallyResponse,omitempty"`
}

// PollAndWriteJobs long-polls the cloud for ERP→Tally jobs, writes them to Tally,
// and reports the result back. Runs in a tight loop until ctx is cancelled.
func PollAndWriteJobs(ctx context.Context, client *tally.Client, db *store.Store, cloudURL, token, companyID string) {
	httpClient := &http.Client{Timeout: 40 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		jobs, err := pollJobs(ctx, httpClient, cloudURL, token, companyID)
		if err != nil {
			// Don't spam logs on network hiccups — just wait and retry
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, job := range jobs {
			result := executeJob(ctx, client, db, job)
			if reportErr := reportResult(ctx, httpClient, cloudURL, token, companyID, result); reportErr != nil {
				fmt.Printf("Reverse sync: failed to report job result %s: %v\n", job.JobID, reportErr)
			}
		}
	}
}

// pollJobs calls GET /connector/jobs/pending with a 30-second timeout.
func pollJobs(ctx context.Context, httpClient *http.Client, cloudURL, token, companyID string) ([]pendingJob, error) {
	url := fmt.Sprintf("%s/connector/jobs/pending?companyId=%s&timeout=30", cloudURL, companyID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jobs/pending returned %d: %s", resp.StatusCode, string(body))
	}

	var result pendingJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode jobs response: %w", err)
	}
	return result.Jobs, nil
}

// executeJob converts the job payload to Tally XML and writes it.
func executeJob(ctx context.Context, client *tally.Client, db *store.Store, job pendingJob) jobResultBody {
	companyName := db.GetConfig("company_tally_name")

	// Marshal payload back to JSON then decode as TallyVoucherInput
	payloadJSON, err := json.Marshal(job.Payload)
	if err != nil {
		return jobResultBody{JobID: job.JobID, Status: "ERROR", ErrorMessage: "marshal payload: " + err.Error()}
	}

	var input TallyVoucherInput
	if err := json.Unmarshal(payloadJSON, &input); err != nil {
		return jobResultBody{JobID: job.JobID, Status: "ERROR", ErrorMessage: "unmarshal TallyVoucherInput: " + err.Error()}
	}

	// Build writer.VoucherInput from TallyVoucherInput
	voucherInput, err := toWriterVoucherInput(input)
	if err != nil {
		return jobResultBody{JobID: job.JobID, Status: "ERROR", ErrorMessage: "build VoucherInput: " + err.Error()}
	}
	voucherInput.CompanyName = companyName

	// Build XML — BuildVoucher returns the inner VOUCHER element only;
	// wrap it in a full Import envelope so we can POST directly.
	xmlBody, err := writer.BuildVoucher(voucherInput)
	if err != nil {
		return jobResultBody{JobID: job.JobID, Status: "ERROR", ErrorMessage: "BuildVoucher: " + err.Error()}
	}

	respBytes, err := client.Post(ctx, xmlBody)
	if err != nil {
		return jobResultBody{JobID: job.JobID, Status: "ERROR", ErrorMessage: "Tally import HTTP: " + err.Error()}
	}
	respXML := string(respBytes)

	tallyResult, err := parser.ParseTallyError(respXML)
	if err != nil || tallyResult.Errors > 0 || (tallyResult.LineError != "" && tallyResult.Created == 0 && tallyResult.Altered == 0) {
		errMsg := fmt.Sprintf("Tally error: %s", tallyResult.LineError)
		if err != nil {
			errMsg = err.Error()
		}
		fmt.Printf("Reverse sync: Tally rejected job %s: %s\n", job.JobID, errMsg)
		return jobResultBody{
			JobID: job.JobID, Status: "ERROR",
			ErrorMessage: errMsg, TallyResponse: respXML,
		}
	}

	action := writer.WriteAction(job.Action)
	switch action {
	case writer.ActionCreate:
		fmt.Printf("Reverse sync: created %s in Tally (job %s, guid %s)\n", job.VoucherType, job.JobID, input.GUID)
	case writer.ActionAlter:
		fmt.Printf("Reverse sync: altered %s in Tally (job %s, guid %s)\n", job.VoucherType, job.JobID, input.GUID)
	case writer.ActionCancel:
		fmt.Printf("Reverse sync: cancelled %s in Tally (job %s, guid %s)\n", job.VoucherType, job.JobID, input.GUID)
	}

	return jobResultBody{
		JobID: job.JobID, Status: "SUCCESS",
		TallyGUID: input.GUID, TallyResponse: respXML,
	}
}

// toWriterVoucherInput converts TallyVoucherInput (from JSON) to writer.VoucherInput.
func toWriterVoucherInput(in TallyVoucherInput) (writer.VoucherInput, error) {
	date, err := time.Parse("20060102", in.Date)
	if err != nil {
		return writer.VoucherInput{}, fmt.Errorf("parse date %q: %w", in.Date, err)
	}

	action := writer.ActionCreate
	switch in.Action {
	case "ALTER":
		action = writer.ActionAlter
	case "CANCEL":
		action = writer.ActionCancel
	}

	ledgers := make([]writer.LedgerEntryInput, 0, len(in.LedgerEntries))
	for _, le := range in.LedgerEntries {
		bills := make([]writer.BillAllocationInput, 0, len(le.BillAllocations))
		for _, ba := range le.BillAllocations {
			var dueDate time.Time
			if ba.DueDate != "" {
				dueDate, _ = time.Parse("20060102", ba.DueDate)
			}
			bills = append(bills, writer.BillAllocationInput{
				Name:     ba.Name,
				BillType: ba.BillType,
				Amount:   ba.Amount,
				DueDate:  dueDate,
			})
		}
		ledgers = append(ledgers, writer.LedgerEntryInput{
			LedgerName:       le.LedgerName,
			IsDeemedPositive: le.IsDeemedPositive,
			IsPartyLedger:    le.IsPartyLedger,
			Amount:           le.Amount,
			BillAllocations:  bills,
		})
	}

	inventory := make([]writer.InventoryEntryInput, 0, len(in.InventoryEntries))
	for _, ie := range in.InventoryEntries {
		inventory = append(inventory, writer.InventoryEntryInput{
			StockItemName: ie.StockItemName,
			GodownName:    "",
			Rate:          ie.Rate,
			RateUOM:       ie.RateUOM,
			Amount:        ie.Amount,
			BilledQty:     ie.BilledQty,
			BilledQtyUOM:  ie.BilledQtyUOM,
			HSNCode:       ie.HSNCode,
			AccountLedger: ie.AccountLedger,
		})
	}

	var refDate time.Time
	if in.ReferenceDate != "" {
		refDate, _ = time.Parse("20060102", in.ReferenceDate)
	}

	return writer.VoucherInput{
		Action:           action,
		GUID:             in.GUID,
		VoucherType:      in.VoucherType,
		ObjView:          in.ObjView,
		Date:             date,
		VoucherNumber:    in.VoucherNumber,
		ReferenceNumber:  in.ReferenceNumber,
		ReferenceDate:    refDate,
		PartyLedgerName:  in.PartyLedgerName,
		Narration:        in.Narration,
		IsInvoice:        in.IsInvoice,
		PlaceOfSupply:    in.PlaceOfSupply,
		IsRCMApplicable:  in.IsRCMApplicable,
		LedgerEntries:    ledgers,
		InventoryEntries: inventory,
	}, nil
}

// reportResult POSTs the job outcome to POST /connector/sync-result.
func reportResult(ctx context.Context, httpClient *http.Client, cloudURL, token, companyID string, result jobResultBody) error {
	result.CompanyID = companyID
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cloudURL+"/connector/sync-result", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync-result returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

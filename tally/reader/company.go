// Package reader contains typed Tally API response readers.
// Each file corresponds to one or more Tally Collection types.
// Reader functions call client.Post, parse the XML response, and return
// clean typed structs — they never return raw XML to callers.
package reader

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/tallybridge/connector/tally"
)


// ---- Typed result structs ---------------------------------------------------

// Company holds the fields returned by API 1 (Company Collection).
// Used for both the company list and the heartbeat health check.
type Company struct {
	Name         string    // GUID-stable display name
	StartingFrom time.Time // books-from date (financial year start)
	Books        time.Time // currently open/active books date
	GUID         string
}

// ---- XML decode targets -----------------------------------------------------

// Collection API response: ENVELOPE/BODY/DATA/COLLECTION/COMPANY
type companyEnvelope struct {
	XMLName xml.Name    `xml:"ENVELOPE"`
	Body    companyBody `xml:"BODY"`
}

type companyBody struct {
	Data companyData `xml:"DATA"`
}

type companyData struct {
	Collection companyCollection `xml:"COLLECTION"`
}

type companyCollection struct {
	Companies []xmlCompany `xml:"COMPANY"`
}

type xmlCompany struct {
	Name string `xml:"NAME,attr"` // NAME is an XML attribute on <COMPANY>
}

// ---- Public API -------------------------------------------------------------

// ListCompanies performs API 1: fetches all open companies from Tally.
// Uses the Collection API with TYPE=Company (same format as Postman collection).
// ISINITIALIZE="Yes" is required for the Company type — unlike other masters.
func ListCompanies(ctx context.Context, c *tally.Client) ([]Company, error) {
	xmlBody := `<?xml version="1.0" encoding="utf-8"?>
<ENVELOPE>
  <HEADER>
    <VERSION>1</VERSION>
    <TALLYREQUEST>Export</TALLYREQUEST>
    <TYPE>Collection</TYPE>
    <ID>TBCompanies</ID>
  </HEADER>
  <BODY>
    <DESC>
      <STATICVARIABLES>
        <SVEXPORTFORMAT>$$SysName:XML</SVEXPORTFORMAT>
      </STATICVARIABLES>
      <TDL><TDLMESSAGE>
        <COLLECTION ISMODIFY="No" ISFIXED="No" ISINITIALIZE="Yes" ISOPTION="No" ISINTERNAL="No" NAME="TBCompanies">
          <TYPE>Company</TYPE>
          <NATIVEMETHOD>Name</NATIVEMETHOD>
        </COLLECTION>
      </TDLMESSAGE></TDL>
    </DESC>
  </BODY>
</ENVELOPE>`

	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.ListCompanies: %w", err)
	}

	return parseCompanyResponse(raw)
}

// HeartbeatInfo holds the result of a lightweight Tally health check.
type HeartbeatInfo struct {
	Online      bool
	OpenCompany string // name of the currently active company, "" if none
	Version     string // Tally version string from ENVELOPE/HEADER if present
	CheckedAt   time.Time
}

// Heartbeat performs a lightweight health check against Tally port 9000.
// It uses ListCompanies internally; a successful response with ≥1 company
// means Tally is ONLINE. The caller should use a short context (≤5 s).
func Heartbeat(ctx context.Context, c *tally.Client) HeartbeatInfo {
	info := HeartbeatInfo{CheckedAt: time.Now().UTC()}

	companies, err := ListCompanies(ctx, c)
	if err != nil {
		info.Online = false
		return info
	}

	info.Online = true
	if len(companies) > 0 {
		// Tally typically returns the active company first.
		info.OpenCompany = companies[0].Name
	}
	return info
}

// ---- Internal parsing -------------------------------------------------------

func parseCompanyResponse(raw []byte) ([]Company, error) {
	var env companyEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("reader.parseCompanyResponse: xml unmarshal: %w", err)
	}

	var out []Company
	for _, xc := range env.Body.Data.Collection.Companies {
		if xc.Name != "" {
			out = append(out, Company{Name: xc.Name})
		}
	}
	return out, nil
}

package parser

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// tallyImportResult parses ENVELOPE > BODY > DATA > IMPORTRESULT (Tally PRIME/ERP9 import response).
type tallyImportResult struct {
	XMLName xml.Name `xml:"ENVELOPE"`
	Body    struct {
		Data struct {
			Result struct {
				Created    int    `xml:"CREATED"`
				Altered    int    `xml:"ALTERED"`
				Errors     int    `xml:"ERRORS"`
				Exceptions int    `xml:"EXCEPTIONS"`
				LineError  string `xml:"LINEERROR"`
			} `xml:"IMPORTRESULT"`
		} `xml:"DATA"`
	} `xml:"BODY"`
}

// tallyResponseLegacy parses a flat <RESPONSE> element (older Tally format).
type tallyResponseLegacy struct {
	XMLName   xml.Name `xml:"RESPONSE"`
	Created   int      `xml:"CREATED"`
	Altered   int      `xml:"ALTERED"`
	Errors    int      `xml:"ERRORS"`
	LineError string   `xml:"LINEERROR"`
}

// TallyWriteResult holds the parsed result of a Tally XML import operation.
type TallyWriteResult struct {
	Created    int
	Altered    int
	Errors     int
	Exceptions int
	LineError  string
}

// ParseTallyError parses the XML body returned by Tally after an import
// (CREATE / ALTER / CANCEL) request.
//
// Handles both the modern ENVELOPE/BODY/DATA/IMPORTRESULT format and
// the legacy flat <RESPONSE> format.
func ParseTallyError(xmlBody string) (TallyWriteResult, error) {
	xmlBody = strings.TrimSpace(xmlBody)
	if xmlBody == "" {
		return TallyWriteResult{}, fmt.Errorf("ParseTallyError: empty response body")
	}

	// Try modern ENVELOPE format first (Tally PRIME / ERP9)
	if strings.Contains(xmlBody, "<ENVELOPE") {
		var env tallyImportResult
		if err := xml.Unmarshal([]byte(xmlBody), &env); err == nil {
			r := env.Body.Data.Result
			return TallyWriteResult{
				Created:    r.Created,
				Altered:    r.Altered,
				Errors:     r.Errors,
				Exceptions: r.Exceptions,
				LineError:  strings.TrimSpace(r.LineError),
			}, nil
		}
	}

	// Fall back to legacy <RESPONSE> format
	var resp tallyResponseLegacy
	if err := xml.Unmarshal([]byte(xmlBody), &resp); err != nil {
		return TallyWriteResult{}, fmt.Errorf("ParseTallyError: xml unmarshal: %w", err)
	}

	return TallyWriteResult{
		Created:   resp.Created,
		Altered:   resp.Altered,
		Errors:    resp.Errors,
		LineError: strings.TrimSpace(resp.LineError),
	}, nil
}

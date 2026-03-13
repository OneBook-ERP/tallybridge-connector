// Package tally contains the Tally XML API client and envelope builder.
// All requests to Tally port 9000 go through this package.
package tally

import (
	"fmt"
	"strings"
	"time"
)

// ---- Static variable helpers ------------------------------------------------

// StaticVars holds optional variables embedded in the request DESC block.
type StaticVars struct {
	Company    string    // SVCURRENTCOMPANY — required for most queries
	FromDate   time.Time // SVFROMDATE — optional, zero = omit
	ToDate     time.Time // SVTODATE   — optional, zero = omit
	ExportFmt  bool      // whether to include SVEXPORTFORMAT (default true for exports)
}

// ---- Collection request builder ---------------------------------------------

// CollectionRequest describes a TDL Collection query.
type CollectionRequest struct {
	Name    string   // Collection name used in HEADER ID
	Type    string   // TYPE attribute on COLLECTION element
	Fetch   []string // FETCH fields
	Filter  string   // optional filter expression, e.g. $$IsAlteredSince:...
	Vars    StaticVars
}

// BuildCollection returns the full XML envelope for an Export/Collection request.
func BuildCollection(req CollectionRequest) string {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString("\n<ENVELOPE>")
	b.WriteString("\n  <HEADER>")
	b.WriteString("\n    <VERSION>1</VERSION>")
	b.WriteString("\n    <TALLYREQUEST>Export</TALLYREQUEST>")
	b.WriteString("\n    <TYPE>Collection</TYPE>")
	fmt.Fprintf(&b, "\n    <ID>%s</ID>", xmlEscape(req.Name))
	b.WriteString("\n  </HEADER>")
	b.WriteString("\n  <BODY>")
	b.WriteString("\n    <DESC>")
	b.WriteString(buildStaticVars(req.Vars))
	b.WriteString("\n      <TDL><TDLMESSAGE>")
	// COLLECTION must have TYPE as a child element (not an attribute) and use
	// NATIVEMETHOD for field selection. The extra boolean attributes are required
	// by Tally ERP 9 for the Collection API to return data.
	fmt.Fprintf(&b, "\n        <COLLECTION ISMODIFY=\"No\" ISFIXED=\"No\" ISINITIALIZE=\"No\" ISOPTION=\"No\" ISINTERNAL=\"No\" NAME=\"%s\">",
		xmlEscape(req.Name))
	fmt.Fprintf(&b, "\n          <TYPE>%s</TYPE>", xmlEscape(req.Type))

	for _, f := range req.Fetch {
		fmt.Fprintf(&b, "\n          <NATIVEMETHOD>%s</NATIVEMETHOD>", xmlEscape(f))
	}

	if req.Filter != "" {
		filterName := xmlEscape(req.Name) + "Filter"
		fmt.Fprintf(&b, "\n          <FILTERS>%s</FILTERS>", filterName)
		b.WriteString("\n        </COLLECTION>")
		fmt.Fprintf(&b, "\n        <SYSTEM TYPE=\"Formulae\" NAME=\"%s\">%s</SYSTEM>", filterName, xmlEscape(req.Filter))
	} else {
		b.WriteString("\n        </COLLECTION>")
	}
	b.WriteString("\n      </TDLMESSAGE></TDL>")
	b.WriteString("\n    </DESC>")
	b.WriteString("\n  </BODY>")
	b.WriteString("\n</ENVELOPE>")

	return b.String()
}

// ---- Object request builder -------------------------------------------------

// ObjectRequest describes a TDL Object (single record) query.
type ObjectRequest struct {
	SubType string     // e.g. "Voucher"
	IDType  string     // e.g. "GUID"
	IDValue string     // the GUID or name value
	Vars    StaticVars
}

// BuildObject returns the full XML envelope for an Export/Object request
// (used for API 11: single voucher by GUID).
func BuildObject(req ObjectRequest) string {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString("\n<ENVELOPE>")
	b.WriteString("\n  <HEADER>")
	b.WriteString("\n    <VERSION>1</VERSION>")
	b.WriteString("\n    <TALLYREQUEST>Export</TALLYREQUEST>")
	b.WriteString("\n    <TYPE>Object</TYPE>")
	fmt.Fprintf(&b, "\n    <SUBTYPE>%s</SUBTYPE>", xmlEscape(req.SubType))
	fmt.Fprintf(&b, "\n    <ID TYPE=\"%s\">%s</ID>",
		xmlEscape(req.IDType), xmlEscape(req.IDValue))
	b.WriteString("\n  </HEADER>")
	b.WriteString("\n  <BODY>")
	b.WriteString("\n    <DESC>")
	b.WriteString(buildStaticVars(req.Vars))
	b.WriteString("\n    </DESC>")
	b.WriteString("\n  </BODY>")
	b.WriteString("\n</ENVELOPE>")

	return b.String()
}

// ---- Import request builder -------------------------------------------------

// ImportRequest describes a TDL Data import operation (create/alter/cancel).
type ImportRequest struct {
	Type    string // e.g. "Vouchers", "Masters"
	Company string // SVCURRENTCOMPANY — targets a specific open company
	Body    string // inner XML — the <TALLYMESSAGE> content, caller-supplied
}

// BuildImport returns the full XML envelope for an Import Data request.
// Uses the IMPORTDATA/REQUESTDESC/REQUESTDATA format which correctly
// respects SVCURRENTCOMPANY (the old Import/VERSION/TYPE/ID format does not).
func BuildImport(req ImportRequest) string {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString("\n<ENVELOPE>")
	b.WriteString("\n  <HEADER>")
	b.WriteString("\n    <TALLYREQUEST>Import Data</TALLYREQUEST>")
	b.WriteString("\n  </HEADER>")
	b.WriteString("\n  <BODY>")
	b.WriteString("\n    <IMPORTDATA>")
	b.WriteString("\n      <REQUESTDESC>")
	fmt.Fprintf(&b, "\n        <REPORTNAME>%s</REPORTNAME>", xmlEscape(req.Type))
	b.WriteString("\n        <STATICVARIABLES>")
	b.WriteString("\n          <SVCURRENTCOMPANY>")
	b.WriteString(xmlEscape(req.Company))
	b.WriteString("</SVCURRENTCOMPANY>")
	b.WriteString("\n        </STATICVARIABLES>")
	b.WriteString("\n      </REQUESTDESC>")
	b.WriteString("\n      <REQUESTDATA>")
	b.WriteString("\n        <TALLYMESSAGE xmlns:UDF=\"TallyUDF\">")
	b.WriteString("\n          ")
	b.WriteString(req.Body)
	b.WriteString("\n        </TALLYMESSAGE>")
	b.WriteString("\n      </REQUESTDATA>")
	b.WriteString("\n    </IMPORTDATA>")
	b.WriteString("\n  </BODY>")
	b.WriteString("\n</ENVELOPE>")

	return b.String()
}

// ---- Helpers ----------------------------------------------------------------

// buildStaticVars renders the STATICVARIABLES block.
func buildStaticVars(v StaticVars) string {
	var b strings.Builder
	b.WriteString("\n      <STATICVARIABLES>")
	// Always include export format for collection queries.
	b.WriteString("\n        <SVEXPORTFORMAT>$$SysName:XML</SVEXPORTFORMAT>")

	if v.Company != "" {
		fmt.Fprintf(&b, "\n        <SVCURRENTCOMPANY>%s</SVCURRENTCOMPANY>",
			xmlEscape(v.Company))
	}
	if !v.FromDate.IsZero() {
		fmt.Fprintf(&b, "\n        <SVFROMDATE TYPE=\"Date\">%s</SVFROMDATE>",
			formatTallyDisplayDate(v.FromDate))
	}
	if !v.ToDate.IsZero() {
		fmt.Fprintf(&b, "\n        <SVTODATE TYPE=\"Date\">%s</SVTODATE>",
			formatTallyDisplayDate(v.ToDate))
	}
	b.WriteString("\n      </STATICVARIABLES>")
	return b.String()
}

// formatTallyDisplayDate formats a date as "1-Apr-2006" for STATICVARIABLES.
func formatTallyDisplayDate(t time.Time) string {
	// Tally accepts "1-Jan-2006" style dates in static variables.
	return t.UTC().Format("2-Jan-2006")
}

// xmlEscape replaces the five XML special characters.
// We avoid encoding/xml here to keep this zero-dependency within the package.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

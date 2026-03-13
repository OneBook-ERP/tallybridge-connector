package tally

import (
	"strings"
	"testing"
	"time"
)

func TestBuildCollection_Basic(t *testing.T) {
	req := CollectionRequest{
		Name:  "List of Companies",
		Type:  "Company",
		Fetch: []string{"Name", "GUID"},
		Vars:  StaticVars{},
	}
	got := BuildCollection(req)

	assertContains(t, got, "<TALLYREQUEST>Export</TALLYREQUEST>")
	assertContains(t, got, "<TYPE>Collection</TYPE>")
	assertContains(t, got, "<ID>List of Companies</ID>")
	assertContains(t, got, `TYPE="Company"`)
	assertContains(t, got, "<FETCH>Name</FETCH>")
	assertContains(t, got, "<FETCH>GUID</FETCH>")
	assertContains(t, got, "$$SysName:XML")
}

func TestBuildCollection_WithCompanyAndDates(t *testing.T) {
	from := time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)

	req := CollectionRequest{
		Name:  "Vouchers",
		Type:  "Voucher",
		Fetch: []string{"GUID", "DATE"},
		Filter: "$$IsAlteredSince:20240401000000",
		Vars: StaticVars{
			Company:  "Demo Company",
			FromDate: from,
			ToDate:   to,
		},
	}
	got := BuildCollection(req)

	assertContains(t, got, "<SVCURRENTCOMPANY>Demo Company</SVCURRENTCOMPANY>")
	assertContains(t, got, ">1-Apr-2024<")
	assertContains(t, got, ">31-Mar-2025<")
	assertContains(t, got, `FILTERS="$$IsAlteredSince:20240401000000"`)
}

func TestBuildCollection_XMLEscaping(t *testing.T) {
	req := CollectionRequest{
		Name:  "A &amp; B",
		Type:  "Company",
		Fetch: []string{"Name"},
		Vars:  StaticVars{Company: `<Corp & "Ltd">`},
	}
	got := BuildCollection(req)

	// Name should be escaped in ID tag
	assertContains(t, got, "A &amp;amp; B") // double-escaped because we escape before writing
	// Company name must be escaped
	assertContains(t, got, "&lt;Corp &amp; &quot;Ltd&quot;&gt;")
}

func TestBuildObject(t *testing.T) {
	req := ObjectRequest{
		SubType: "Voucher",
		IDType:  "GUID",
		IDValue: "abc-123-def",
		Vars:    StaticVars{},
	}
	got := BuildObject(req)

	assertContains(t, got, "<TALLYREQUEST>Export</TALLYREQUEST>")
	assertContains(t, got, "<TYPE>Object</TYPE>")
	assertContains(t, got, "<SUBTYPE>Voucher</SUBTYPE>")
	assertContains(t, got, `<ID TYPE="GUID">abc-123-def</ID>`)
}

func TestBuildImport(t *testing.T) {
	req := ImportRequest{
		Type: "Vouchers",
		Body: `<VOUCHER VCHTYPE="Sales"></VOUCHER>`,
	}
	got := BuildImport(req)

	assertContains(t, got, "<TALLYREQUEST>Import</TALLYREQUEST>")
	assertContains(t, got, "<TYPE>Data</TYPE>")
	assertContains(t, got, "<ID>Vouchers</ID>")
	assertContains(t, got, `<VOUCHER VCHTYPE="Sales"></VOUCHER>`)
}

// assertContains is a helper that fails the test if needle is absent from haystack.
func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q\ngot:\n%s", needle, haystack)
	}
}

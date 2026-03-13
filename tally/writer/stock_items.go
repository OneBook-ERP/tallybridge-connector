package writer

import (
	"fmt"
	"strings"

	"github.com/tallybridge/connector/tally"
)

// StockItemInput holds the data needed to create a stock item master (API 17).
type StockItemInput struct {
	Action                WriteAction
	Name                  string
	Parent                string
	BaseUnits             string
	GUID                  string
	HSNCode               string
	GSTApplicable         string
	TaxClassificationName string
}

// BuildStockItem returns the full import XML envelope for a stock item create (API 17).
func BuildStockItem(si StockItemInput) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<STOCKITEM ACTION="%s">`, si.Action)
	fmt.Fprintf(&b, "\n  <NAME>%s</NAME>", xmlEsc(si.Name))
	fmt.Fprintf(&b, "\n  <PARENT>%s</PARENT>", xmlEsc(si.Parent))
	if si.BaseUnits != "" {
		fmt.Fprintf(&b, "\n  <BASEUNITS>%s</BASEUNITS>", xmlEsc(si.BaseUnits))
	}
	fmt.Fprintf(&b, "\n  <GUID>%s</GUID>", xmlEsc(si.GUID))
	if si.HSNCode != "" {
		fmt.Fprintf(&b, "\n  <HSNCODE>%s</HSNCODE>", xmlEsc(si.HSNCode))
	}
	if si.GSTApplicable != "" {
		fmt.Fprintf(&b, "\n  <GSTAPPLICABLE>%s</GSTAPPLICABLE>", xmlEsc(si.GSTApplicable))
	}
	if si.TaxClassificationName != "" {
		fmt.Fprintf(&b, "\n  <TAXCLASSIFICATIONNAME>%s</TAXCLASSIFICATIONNAME>",
			xmlEsc(si.TaxClassificationName))
	}
	b.WriteString("\n</STOCKITEM>")

	return tally.BuildImport(tally.ImportRequest{
		Type: "All Masters",
		Body: b.String(),
	})
}

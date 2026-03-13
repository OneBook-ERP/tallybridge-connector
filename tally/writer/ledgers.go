package writer

import (
	"fmt"
	"strings"

	"github.com/tallybridge/connector/tally"
)

// LedgerInput holds the data needed to create or alter a ledger master (API 16).
type LedgerInput struct {
	Action              WriteAction
	Name                string
	Parent              string // "Sundry Debtors" | "Sundry Creditors" | group name
	GUID                string
	MailingName         string
	Address             []string // multi-line
	PinCode             string
	StateName           string
	CountryName         string
	PanItNo             string
	GSTRegistrationType string
	PartyGSTIN          string
	IsBillWiseOn        bool
	CreditPeriod        string
}

// BuildLedger returns the full import XML envelope for a ledger create/alter (API 16).
func BuildLedger(l LedgerInput) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<LEDGER ACTION="%s">`, l.Action)
	fmt.Fprintf(&b, "\n  <NAME>%s</NAME>", xmlEsc(l.Name))
	fmt.Fprintf(&b, "\n  <PARENT>%s</PARENT>", xmlEsc(l.Parent))
	fmt.Fprintf(&b, "\n  <GUID>%s</GUID>", xmlEsc(l.GUID))
	if l.MailingName != "" {
		fmt.Fprintf(&b, "\n  <MAILINGNAME>%s</MAILINGNAME>", xmlEsc(l.MailingName))
	}
	// Multi-line address
	if len(l.Address) > 0 {
		b.WriteString("\n  <ADDRESS.LIST>")
		for _, line := range l.Address {
			fmt.Fprintf(&b, "\n    <ADDR>%s</ADDR>", xmlEsc(line))
		}
		b.WriteString("\n  </ADDRESS.LIST>")
	}
	if l.PinCode != "" {
		fmt.Fprintf(&b, "\n  <PINCODE>%s</PINCODE>", xmlEsc(l.PinCode))
	}
	if l.StateName != "" {
		fmt.Fprintf(&b, "\n  <LEDSTATENAME>%s</LEDSTATENAME>", xmlEsc(l.StateName))
	}
	if l.CountryName != "" {
		fmt.Fprintf(&b, "\n  <COUNTRYNAME>%s</COUNTRYNAME>", xmlEsc(l.CountryName))
	}
	if l.PanItNo != "" {
		fmt.Fprintf(&b, "\n  <PANITNO>%s</PANITNO>", xmlEsc(l.PanItNo))
	}
	if l.GSTRegistrationType != "" {
		fmt.Fprintf(&b, "\n  <GSTREGISTRATIONTYPE>%s</GSTREGISTRATIONTYPE>",
			xmlEsc(l.GSTRegistrationType))
	}
	if l.PartyGSTIN != "" {
		fmt.Fprintf(&b, "\n  <PARTYGSTIN>%s</PARTYGSTIN>", xmlEsc(l.PartyGSTIN))
	}
	if l.IsBillWiseOn {
		b.WriteString("\n  <ISBILLWISEON>Yes</ISBILLWISEON>")
	}
	if l.CreditPeriod != "" {
		fmt.Fprintf(&b, "\n  <CREDITPERIOD>%s</CREDITPERIOD>", xmlEsc(l.CreditPeriod))
	}
	b.WriteString("\n</LEDGER>")

	return tally.BuildImport(tally.ImportRequest{
		Type: "All Masters",
		Body: b.String(),
	})
}

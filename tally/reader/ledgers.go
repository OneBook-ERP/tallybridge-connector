package reader

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/parser"
)

// Ledger represents a Tally ledger master (API 3).
type Ledger struct {
	Name                 string
	Parent               string
	GUID                 string
	MailingName          string
	Address              []string // multi-line — ADDRESS.LIST joined
	PinCode              string
	CountryName          string
	StateName            string
	Email                string
	Mobile               string
	PanItNo              string
	GSTRegistrationType  string
	PartyGSTIN           string
	IsBillWiseOn         bool
	CreditPeriod         string
	OpeningBalance       float64
	IsTDSApplicable      bool
	TDSNatureOfPayment   string
	BankAccountNumber    string
	IFSCode              string
	AlteredOn            time.Time
}

type ledgersEnvelope struct {
	XMLName xml.Name    `xml:"ENVELOPE"`
	Body    ledgersBody `xml:"BODY"`
}
type ledgersBody struct{ Data ledgersData `xml:"DATA"` }
type ledgersData struct{ Collection ledgersCollection `xml:"COLLECTION"` }
type ledgersCollection struct{ Ledgers []xmlLedger `xml:"LEDGER"` }

type xmlLedger struct {
	Name                string   `xml:"NAME,attr"` // Tally returns NAME as an XML attribute on <LEDGER>
	Parent              string   `xml:"PARENT"`
	GUID                string   `xml:"GUID"`
	MailingName         string   `xml:"MAILINGNAME"`
	Address             []string `xml:"ADDRESS>ADDR"`
	PinCode             string   `xml:"PINCODE"`
	CountryName         string   `xml:"COUNTRYNAME"`
	LedStateName        string   `xml:"LEDSTATENAME"`
	Email               string   `xml:"EMAIL"`
	LedgerMobile        string   `xml:"LEDGERMOBILE"`
	PanItNo             string   `xml:"PANITNO"`
	GSTRegistrationType string   `xml:"GSTREGISTRATIONTYPE"`
	PartyGSTIN          string   `xml:"PARTYGSTIN"`
	IsBillWiseOn        string   `xml:"ISBILLWISEON"`
	CreditPeriod        string   `xml:"CREDITPERIOD"`
	OpeningBalance      string   `xml:"OPENINGBALANCE"`
	IsTDSApplicable     string   `xml:"ISTDSAPPLICABLE"`
	TDSNatureOfPayment  string   `xml:"TDSNATUREOFPAYMENT"`
	BankAccountNumber   string   `xml:"BANKACCOUNTNUMBER"`
	IFSCode             string   `xml:"IFSCODE"`
	AlteredOn           string   `xml:"ALTEREDON"`
}

var ledgerFetchFields = []string{
	"Name", "Parent", "GUID", "MailingName", "Address", "PinCode",
	"CountryName", "LedStateName", "Email", "LedgerMobile", "PanItNo",
	"GSTRegistrationType", "PartyGSTIN", "IsBillWiseOn", "CreditPeriod",
	"OpeningBalance", "IsTDSApplicable", "TDSNatureOfPayment",
	"BankAccountNumber", "IFSCode", "ALTEREDON",
}

// ListLedgers fetches all ledger masters from Tally (API 3).
func ListLedgers(ctx context.Context, c *tally.Client, company string) ([]Ledger, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBLedgers",
		Type:  "Ledger",
		Fetch: ledgerFetchFields,
		Vars:  tally.StaticVars{Company: company},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.ListLedgers: %w", err)
	}
	var env ledgersEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("reader.ListLedgers: xml: %w", err)
	}
	out := make([]Ledger, 0, len(env.Body.Data.Collection.Ledgers))
	for _, xl := range env.Body.Data.Collection.Ledgers {
		l := Ledger{
			Name:                xl.Name,
			Parent:              xl.Parent,
			GUID:                xl.GUID,
			MailingName:         xl.MailingName,
			Address:             xl.Address,
			PinCode:             xl.PinCode,
			CountryName:         xl.CountryName,
			StateName:           xl.LedStateName,
			Email:               xl.Email,
			Mobile:              xl.LedgerMobile,
			PanItNo:             xl.PanItNo,
			GSTRegistrationType: xl.GSTRegistrationType,
			PartyGSTIN:          xl.PartyGSTIN,
			IsBillWiseOn:        parser.ParseBool(xl.IsBillWiseOn),
			CreditPeriod:        xl.CreditPeriod,
			OpeningBalance:      parser.ParseAmount(xl.OpeningBalance),
			IsTDSApplicable:     parser.ParseBool(xl.IsTDSApplicable),
			TDSNatureOfPayment:  xl.TDSNatureOfPayment,
			BankAccountNumber:   xl.BankAccountNumber,
			IFSCode:             xl.IFSCode,
		}
		if t, err := parser.ParseDate(xl.AlteredOn); err == nil {
			l.AlteredOn = t
		}
		out = append(out, l)
	}
	return out, nil
}

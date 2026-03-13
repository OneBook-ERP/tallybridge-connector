package reader

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/parser"
)

// VoucherType represents a Tally voucher type master (API 7).
type VoucherType struct {
	Name                string
	Parent              string
	NatureOfVoucherType string
	IsActive            bool
	IsTaxInvoice        bool
	GUID                string
}

type voucherTypesEnvelope struct {
	XMLName xml.Name         `xml:"ENVELOPE"`
	Body    voucherTypesBody `xml:"BODY"`
}
type voucherTypesBody struct{ Data voucherTypesData `xml:"DATA"` }
type voucherTypesData struct{ Collection voucherTypesCollection `xml:"COLLECTION"` }
type voucherTypesCollection struct{ VoucherTypes []xmlVoucherType `xml:"VOUCHERTYPE"` }

type xmlVoucherType struct {
	Name                string `xml:"NAME,attr"` // NAME is an XML attribute on <VOUCHERTYPE>
	Parent              string `xml:"PARENT"`    // Parent = base type; acts as "nature" (Sales, Purchase, etc.)
	NatureOfVoucherType string `xml:"NATUREOFVOUCHERTYPE"` // may be empty; Tally doesn't always export this
	IsActive            string `xml:"ISACTIVE"`
	IsTaxInvoice        string `xml:"ISTAXTinvoice"`
	GUID                string `xml:"GUID"`
}

// ListVoucherTypes fetches all voucher type masters from Tally (API 7).
func ListVoucherTypes(ctx context.Context, c *tally.Client, company string) ([]VoucherType, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBVoucherTypes",
		Type:  "Voucher Type",
		Fetch: []string{"Name", "Parent", "NatureOfVoucherType", "IsActive", "IsTaxInvoice", "GUID"},
		Vars:  tally.StaticVars{Company: company},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.ListVoucherTypes: %w", err)
	}
	var env voucherTypesEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("reader.ListVoucherTypes: xml: %w", err)
	}
	out := make([]VoucherType, 0, len(env.Body.Data.Collection.VoucherTypes))
	for _, xv := range env.Body.Data.Collection.VoucherTypes {
		out = append(out, VoucherType{
			Name:                xv.Name,
			Parent:              xv.Parent,
			NatureOfVoucherType: xv.NatureOfVoucherType,
			IsActive:            parser.ParseBool(xv.IsActive),
			IsTaxInvoice:        parser.ParseBool(xv.IsTaxInvoice),
			GUID:                xv.GUID,
		})
	}
	return out, nil
}

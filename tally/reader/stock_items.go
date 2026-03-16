package reader

import (
	"context"
	"encoding/xml"
	"fmt"
	"math"
	"time"

	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/parser"
)

// StockItem represents a Tally stock item master (API 5).
type StockItem struct {
	Name                  string
	Parent                string
	BaseUnits             string
	GUID                  string
	HSNCode               string
	GSTApplicable         string
	TaxClassificationName string
	GSTRate               float64
	OpeningBalance        float64 // opening qty
	OpeningRate           float64 // per-unit rate (may be 0 if Tally doesn't have it)
	OpeningValue          float64 // total opening value = qty × rate (use this when OpeningRate is 0)
	AlteredOn             time.Time
}

type stockItemsEnvelope struct {
	XMLName xml.Name       `xml:"ENVELOPE"`
	Body    stockItemsBody `xml:"BODY"`
}
type stockItemsBody struct{ Data stockItemsData `xml:"DATA"` }
type stockItemsData struct{ Collection stockItemsCollection `xml:"COLLECTION"` }
type stockItemsCollection struct{ Items []xmlStockItem `xml:"STOCKITEM"` }

// xmlHSNDetails maps the nested HSNDETAILS.LIST element where Tally stores HSN codes.
// Tally stores HSN in HSNDETAILS.LIST > HSNCODE, NOT in GSTDETAILS.LIST > HSNCODE
// (the latter is consistently empty even when HSN is set in Tally UI).
type xmlHSNDetails struct {
	HSNCode string `xml:"HSNCODE"`
}

type xmlStockItem struct {
	Name                  string          `xml:"NAME,attr"` // NAME is an XML attribute on <STOCKITEM>
	Parent                string          `xml:"PARENT"`
	BaseUnits             string          `xml:"BASEUNITS"`
	GUID                  string          `xml:"GUID"`
	HSNCode               string          `xml:"HSNCODE"` // from GSTDETAILS — usually empty
	HSNDetails            []xmlHSNDetails `xml:"HSNDETAILS.LIST"`
	GSTApplicable         string          `xml:"GSTAPPLICABLE"`
	TaxClassificationName string          `xml:"TAXCLASSIFICATIONNAME"`
	GSTRate               string          `xml:"GSTRATE"`
	OpeningBalance        string          `xml:"OPENINGBALANCE"`
	OpeningRate           string          `xml:"OPENINGRATE"`
	OpeningValue          string          `xml:"OPENINGVALUE"` // total opening value (qty × rate)
	AlteredOn             string          `xml:"ALTEREDON"`
}

var stockItemFetchFields = []string{
	"Name", "Parent", "BaseUnits", "GUID",
	"GSTApplicable", "TaxClassificationName", "GSTRate",
	"OpeningBalance", "OpeningRate", "OpeningValue", "ALTEREDON",
	"*", // wildcard fetches full object including HSNDETAILS.LIST
}

// ListStockItems fetches all stock item masters from Tally (API 5).
// fromDate should be the FY start (e.g. 2023-04-01) so Tally returns the correct
// opening balance for that fiscal year. Without it Tally defaults to the current
// session FY and returns 0 opening balance for prior-year companies.
func ListStockItems(ctx context.Context, c *tally.Client, company string, fromDate, toDate time.Time) ([]StockItem, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBStockItems",
		Type:  "Stock Item",
		Fetch: stockItemFetchFields,
		Vars:  tally.StaticVars{Company: company, FromDate: fromDate, ToDate: toDate},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.ListStockItems: %w", err)
	}
	var env stockItemsEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("reader.ListStockItems: xml: %w", err)
	}
	out := make([]StockItem, 0, len(env.Body.Data.Collection.Items))
	for _, xi := range env.Body.Data.Collection.Items {
		// Prefer HSN from HSNDETAILS.LIST (where Tally actually stores it),
		// fall back to direct HSNCODE field (GSTDETAILS — usually empty).
		hsn := xi.HSNCode
		if len(xi.HSNDetails) > 0 && xi.HSNDetails[0].HSNCode != "" {
			hsn = xi.HSNDetails[0].HSNCode
		}
		openingQty := parser.ParseAmount(xi.OpeningBalance)   // format: "1000.000 KGS" — UOM suffix stripped by ParseAmount
		openingRate, _, _ := parser.ParseRate(xi.OpeningRate) // format: "36.50/KGS" — ParseAmount can't handle slash
		openingValue := math.Abs(parser.ParseAmount(xi.OpeningValue)) // Tally stores as negative (Cr convention); normalise
		// If Tally has total value but not per-unit rate, derive rate from value/qty
		if openingRate == 0 && openingValue != 0 && openingQty != 0 {
			openingRate = openingValue / openingQty
		}
		// If we have rate but not value, derive value
		if openingValue == 0 && openingRate != 0 && openingQty != 0 {
			openingValue = openingQty * openingRate
		}
		item := StockItem{
			Name:                  xi.Name,
			Parent:                xi.Parent,
			BaseUnits:             xi.BaseUnits,
			GUID:                  xi.GUID,
			HSNCode:               hsn,
			GSTApplicable:         xi.GSTApplicable,
			TaxClassificationName: xi.TaxClassificationName,
			GSTRate:               parser.ParseAmount(xi.GSTRate),
			OpeningBalance:        openingQty,
			OpeningRate:           openingRate,
			OpeningValue:          openingValue,
		}
		if t, err := parser.ParseDate(xi.AlteredOn); err == nil {
			item.AlteredOn = t
		}
		out = append(out, item)
	}
	return out, nil
}

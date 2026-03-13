package reader

import (
	"context"
	"encoding/xml"
	"fmt"
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
	OpeningBalance        float64 // qty only; use OpeningBalanceQty for structured
	OpeningRate           float64
	AlteredOn             time.Time
}

type stockItemsEnvelope struct {
	XMLName xml.Name       `xml:"ENVELOPE"`
	Body    stockItemsBody `xml:"BODY"`
}
type stockItemsBody struct{ Data stockItemsData `xml:"DATA"` }
type stockItemsData struct{ Collection stockItemsCollection `xml:"COLLECTION"` }
type stockItemsCollection struct{ Items []xmlStockItem `xml:"STOCKITEM"` }

type xmlStockItem struct {
	Name                  string `xml:"NAME,attr"` // NAME is an XML attribute on <STOCKITEM>
	Parent                string `xml:"PARENT"`
	BaseUnits             string `xml:"BASEUNITS"`
	GUID                  string `xml:"GUID"`
	HSNCode               string `xml:"HSNCODE"`
	GSTApplicable         string `xml:"GSTAPPLICABLE"`
	TaxClassificationName string `xml:"TAXCLASSIFICATIONNAME"`
	GSTRate               string `xml:"GSTRATE"`
	OpeningBalance        string `xml:"OPENINGBALANCE"`
	OpeningRate           string `xml:"OPENINGRATE"`
	AlteredOn             string `xml:"ALTEREDON"`
}

var stockItemFetchFields = []string{
	"Name", "Parent", "BaseUnits", "GUID", "HSNCode",
	"GSTApplicable", "TaxClassificationName", "GSTRate",
	"OpeningBalance", "OpeningRate", "ALTEREDON",
}

// ListStockItems fetches all stock item masters from Tally (API 5).
func ListStockItems(ctx context.Context, c *tally.Client, company string) ([]StockItem, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBStockItems",
		Type:  "Stock Item",
		Fetch: stockItemFetchFields,
		Vars:  tally.StaticVars{Company: company},
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
		item := StockItem{
			Name:                  xi.Name,
			Parent:                xi.Parent,
			BaseUnits:             xi.BaseUnits,
			GUID:                  xi.GUID,
			HSNCode:               xi.HSNCode,
			GSTApplicable:         xi.GSTApplicable,
			TaxClassificationName: xi.TaxClassificationName,
			GSTRate:               parser.ParseAmount(xi.GSTRate),
			OpeningBalance:        parser.ParseAmount(xi.OpeningBalance),
			OpeningRate:           parser.ParseAmount(xi.OpeningRate),
		}
		if t, err := parser.ParseDate(xi.AlteredOn); err == nil {
			item.AlteredOn = t
		}
		out = append(out, item)
	}
	return out, nil
}

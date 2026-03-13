package reader

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/parser"
)

// StockGroup represents a Tally stock group (API 4).
type StockGroup struct {
	Name      string
	Parent    string
	IsAddable bool
	GUID      string
	AlteredOn time.Time
}

type stockGroupsEnvelope struct {
	XMLName xml.Name       `xml:"ENVELOPE"`
	Body    stockGroupsBody `xml:"BODY"`
}
type stockGroupsBody struct{ Data stockGroupsData `xml:"DATA"` }
type stockGroupsData struct{ Collection stockGroupsCollection `xml:"COLLECTION"` }
type stockGroupsCollection struct{ StockGroups []xmlStockGroup `xml:"STOCKGROUP"` }

type xmlStockGroup struct {
	Name      string `xml:"NAME"`
	Parent    string `xml:"PARENT"`
	IsAddable string `xml:"ISADDABLE"`
	GUID      string `xml:"GUID"`
	AlteredOn string `xml:"ALTEREDON"`
}

// ListStockGroups fetches all stock groups from Tally (API 4).
func ListStockGroups(ctx context.Context, c *tally.Client, company string) ([]StockGroup, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBStockGroups",
		Type:  "Stock Group",
		Fetch: []string{"Name", "Parent", "IsAddable", "GUID", "ALTEREDON"},
		Vars:  tally.StaticVars{Company: company},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.ListStockGroups: %w", err)
	}
	var env stockGroupsEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("reader.ListStockGroups: xml: %w", err)
	}
	out := make([]StockGroup, 0, len(env.Body.Data.Collection.StockGroups))
	for _, xs := range env.Body.Data.Collection.StockGroups {
		g := StockGroup{
			Name:      xs.Name,
			Parent:    xs.Parent,
			IsAddable: parser.ParseBool(xs.IsAddable),
			GUID:      xs.GUID,
		}
		if t, err := parser.ParseDate(xs.AlteredOn); err == nil {
			g.AlteredOn = t
		}
		out = append(out, g)
	}
	return out, nil
}

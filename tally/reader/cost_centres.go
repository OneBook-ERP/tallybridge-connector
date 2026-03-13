package reader

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/tallybridge/connector/tally"
)

// CostCentre represents a Tally cost centre (API 9).
type CostCentre struct {
	Name     string
	Parent   string
	Category string
	GUID     string
}

type costCentresEnvelope struct {
	XMLName xml.Name        `xml:"ENVELOPE"`
	Body    costCentresBody `xml:"BODY"`
}
type costCentresBody struct{ Data costCentresData `xml:"DATA"` }
type costCentresData struct{ Collection costCentresCollection `xml:"COLLECTION"` }
type costCentresCollection struct{ CostCentres []xmlCostCentre `xml:"COSTCENTRE"` }

type xmlCostCentre struct {
	Name     string `xml:"NAME"`
	Parent   string `xml:"PARENT"`
	Category string `xml:"CATEGORY"`
	GUID     string `xml:"GUID"`
}

// ListCostCentres fetches all cost centres from Tally (API 9).
func ListCostCentres(ctx context.Context, c *tally.Client, company string) ([]CostCentre, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBCostCentres",
		Type:  "Cost Centre",
		Fetch: []string{"Name", "Parent", "Category", "GUID"},
		Vars:  tally.StaticVars{Company: company},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.ListCostCentres: %w", err)
	}
	var env costCentresEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("reader.ListCostCentres: xml: %w", err)
	}
	out := make([]CostCentre, 0, len(env.Body.Data.Collection.CostCentres))
	for _, xc := range env.Body.Data.Collection.CostCentres {
		out = append(out, CostCentre{
			Name:     xc.Name,
			Parent:   xc.Parent,
			Category: xc.Category,
			GUID:     xc.GUID,
		})
	}
	return out, nil
}

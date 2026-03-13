package reader

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/tallybridge/connector/tally"
)

// Godown represents a Tally godown/warehouse (API 8).
type Godown struct {
	Name       string
	Parent     string
	IsInternal bool
	Address    string
	GUID       string
}

type godownsEnvelope struct {
	XMLName xml.Name     `xml:"ENVELOPE"`
	Body    godownsBody  `xml:"BODY"`
}
type godownsBody struct{ Data godownsData `xml:"DATA"` }
type godownsData struct{ Collection godownsCollection `xml:"COLLECTION"` }
type godownsCollection struct{ Godowns []xmlGodown `xml:"GODOWN"` }

type xmlGodown struct {
	Name       string `xml:"NAME"`
	Parent     string `xml:"PARENT"`
	IsInternal string `xml:"ISINTERNAL"`
	Address    string `xml:"ADDRESS"`
	GUID       string `xml:"GUID"`
}

// ListGodowns fetches all godowns/warehouses from Tally (API 8).
func ListGodowns(ctx context.Context, c *tally.Client, company string) ([]Godown, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBGodowns",
		Type:  "Godown",
		Fetch: []string{"Name", "Parent", "IsInternal", "Address", "GUID"},
		Vars:  tally.StaticVars{Company: company},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.ListGodowns: %w", err)
	}
	var env godownsEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("reader.ListGodowns: xml: %w", err)
	}
	out := make([]Godown, 0, len(env.Body.Data.Collection.Godowns))
	for _, xg := range env.Body.Data.Collection.Godowns {
		out = append(out, Godown{
			Name:       xg.Name,
			Parent:     xg.Parent,
			IsInternal: xg.IsInternal == "Yes",
			Address:    xg.Address,
			GUID:       xg.GUID,
		})
	}
	return out, nil
}

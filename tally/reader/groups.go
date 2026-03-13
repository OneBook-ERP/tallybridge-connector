package reader

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/parser"
)

// Group represents a Tally ledger group (API 2).
type Group struct {
	Name              string
	Parent            string
	PrimaryGroup      string
	IsRevenue         bool
	IsDeemedPositive  bool
	GSTApplicable     string
	GUID              string
	AlteredOn         time.Time
}

type groupsEnvelope struct {
	XMLName xml.Name   `xml:"ENVELOPE"`
	Body    groupsBody `xml:"BODY"`
}
type groupsBody struct {
	Data groupsData `xml:"DATA"`
}
type groupsData struct {
	Collection groupsCollection `xml:"COLLECTION"`
}
type groupsCollection struct {
	Groups []xmlGroup `xml:"GROUP"`
}
type xmlGroup struct {
	Name             string `xml:"NAME,attr"` // NAME is an XML attribute on <GROUP>
	Parent           string `xml:"PARENT"`
	PrimaryGroup     string `xml:"PRIMARYGROUP"`
	IsRevenue        string `xml:"ISREVENUE"`
	IsDeemedPositive string `xml:"ISDEEMEDPOSITIVE"`
	GSTApplicable    string `xml:"GSTAPPLICABLE"`
	GUID             string `xml:"GUID"`
	AlteredOn        string `xml:"ALTEREDON"`
}

var groupFetchFields = []string{
	"Name", "Parent", "PrimaryGroup", "IsRevenue",
	"IsDeemedPositive", "GSTApplicable", "GUID", "ALTEREDON",
}

// ListGroups fetches all ledger groups from Tally (API 2).
func ListGroups(ctx context.Context, c *tally.Client, company string) ([]Group, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBGroups",
		Type:  "Group",
		Fetch: groupFetchFields,
		Vars:  tally.StaticVars{Company: company},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.ListGroups: %w", err)
	}
	var env groupsEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("reader.ListGroups: xml: %w", err)
	}
	out := make([]Group, 0, len(env.Body.Data.Collection.Groups))
	for _, xg := range env.Body.Data.Collection.Groups {
		g := Group{
			Name:             xg.Name,
			Parent:           xg.Parent,
			PrimaryGroup:     xg.PrimaryGroup,
			IsRevenue:        parser.ParseBool(xg.IsRevenue),
			IsDeemedPositive: parser.ParseBool(xg.IsDeemedPositive),
			GSTApplicable:    xg.GSTApplicable,
			GUID:             xg.GUID,
		}
		if t, err := parser.ParseDate(xg.AlteredOn); err == nil {
			g.AlteredOn = t
		}
		out = append(out, g)
	}
	return out, nil
}

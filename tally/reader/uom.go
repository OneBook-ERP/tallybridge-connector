package reader

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/tallybridge/connector/tally"
	"github.com/tallybridge/connector/tally/parser"
)

// UOM represents a Tally unit of measure (API 6).
type UOM struct {
	Name          string
	OriginalName  string
	IsSimpleUnit  bool
	DecimalPlaces int
	BaseUnits     string  // for compound UOM
	Conversion    float64 // for compound UOM
	GUID          string
}

type uomEnvelope struct {
	XMLName xml.Name `xml:"ENVELOPE"`
	Body    uomBody  `xml:"BODY"`
}
type uomBody struct{ Data uomData `xml:"DATA"` }
type uomData struct{ Collection uomCollection `xml:"COLLECTION"` }
type uomCollection struct{ Units []xmlUOM `xml:"UNIT"` }

type xmlUOM struct {
	Name          string `xml:"NAME"`
	OriginalName  string `xml:"ORIGINALNAME"`
	IsSimpleUnit  string `xml:"ISSIMPLEUNIT"`
	DecimalPlaces string `xml:"DECIMALPLACES"`
	BaseUnits     string `xml:"BASEUNITS"`
	Conversion    string `xml:"CONVERSION"`
	GUID          string `xml:"GUID"`
}

// ListUOM fetches all units of measure from Tally (API 6).
func ListUOM(ctx context.Context, c *tally.Client, company string) ([]UOM, error) {
	xmlBody := tally.BuildCollection(tally.CollectionRequest{
		Name:  "TBUnits",
		Type:  "Unit",
		Fetch: []string{"Name", "OriginalName", "IsSimpleUnit", "DecimalPlaces", "BaseUnits", "Conversion", "GUID"},
		Vars:  tally.StaticVars{Company: company},
	})
	raw, err := c.Post(ctx, xmlBody)
	if err != nil {
		return nil, fmt.Errorf("reader.ListUOM: %w", err)
	}
	var env uomEnvelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("reader.ListUOM: xml: %w", err)
	}
	out := make([]UOM, 0, len(env.Body.Data.Collection.Units))
	for _, xu := range env.Body.Data.Collection.Units {
		u := UOM{
			Name:         xu.Name,
			OriginalName: xu.OriginalName,
			IsSimpleUnit: parser.ParseBool(xu.IsSimpleUnit),
			BaseUnits:    xu.BaseUnits,
			GUID:         xu.GUID,
			Conversion:   parser.ParseAmount(xu.Conversion),
		}
		// DecimalPlaces is a small int stored as text by Tally.
		if dp := parser.ParseAmount(xu.DecimalPlaces); dp >= 0 {
			u.DecimalPlaces = int(dp)
		}
		out = append(out, u)
	}
	return out, nil
}

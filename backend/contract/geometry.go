package contract

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// The service area and its zones are one checked-in GeoJSON file, embedded in the binary.
// The backend needs the zone polygons to assign vehicles; the client needs the same
// geometry to draw boundaries and labels. Two copies would be one too many, so the file is
// served verbatim in Config and parsed here for derivation (ADR-0001 §1.9).
//
// It is a hand-authored polygon rather than a bounding box or real city limits. A rectangle
// over Las Vegas contains desert, mountains and the airport — regions that are correctly
// empty and would be flagged as low coverage permanently, which inverts the feature rather
// than weakening it. Zones are named places with their own minimums rather than a uniform
// grid, because the operator's output is spoken: "two cars short downtown" can be said on a
// radio (PRODUCT-SPEC §7.2).

//go:embed service-area.geojson
var serviceAreaGeoJSON []byte

const kindZone = "zone"

// Zone is one named area with its own expectation of how many available vehicles it needs.
// Zones differ, because a busy central zone needs more than a residential edge, and they do
// not tile the service area — a vehicle can be inside the area and in no zone
// (PRODUCT-SPEC §2.5).
type Zone struct {
	ID      ZoneID
	Name    string
	Minimum int
	// Boundary is a closed ring in GeoJSON order.
	Boundary []Point
}

var zones = mustParseZones(serviceAreaGeoJSON)

// Zones returns the zone definitions in the order they appear in the file.
func Zones() []Zone { return zones }

// mustParseZones fails at startup rather than at the first tick. The file is hand authored,
// so the plausible mistake is a zone missing its minimum or its name — which would
// otherwise make that zone quietly always covered, or unnameable on a radio.
func mustParseZones(raw []byte) []Zone {
	var doc struct {
		Features []struct {
			Properties struct {
				Kind    string `json:"kind"`
				ZoneID  ZoneID `json:"zoneId"`
				Name    string `json:"name"`
				Minimum int    `json:"minimumAvailable"`
			} `json:"properties"`
			Geometry struct {
				Type        string    `json:"type"`
				Coordinates [][]Point `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		panic(fmt.Sprintf("contract: service-area.geojson is not valid GeoJSON: %v", err))
	}

	var parsed []Zone
	seen := map[ZoneID]bool{}
	for _, feature := range doc.Features {
		props := feature.Properties
		if props.Kind != kindZone {
			continue
		}
		switch {
		case props.ZoneID == "" || props.ZoneID == ZoneNone:
			panic(fmt.Sprintf("contract: zone %q has no usable zoneId", props.Name))
		case seen[props.ZoneID]:
			panic(fmt.Sprintf("contract: zone %q is defined twice", props.ZoneID))
		case props.Name == "":
			panic(fmt.Sprintf("contract: zone %q has no name", props.ZoneID))
		case props.Minimum < 1:
			panic(fmt.Sprintf("contract: zone %q has no minimumAvailable", props.ZoneID))
		case feature.Geometry.Type != "Polygon" || len(feature.Geometry.Coordinates) != 1:
			panic(fmt.Sprintf("contract: zone %q is not a single-ring Polygon", props.ZoneID))
		}

		ring := feature.Geometry.Coordinates[0]
		if len(ring) < 4 || ring[0] != ring[len(ring)-1] {
			panic(fmt.Sprintf("contract: zone %q is not a closed ring", props.ZoneID))
		}

		seen[props.ZoneID] = true
		parsed = append(parsed, Zone{
			ID:       props.ZoneID,
			Name:     props.Name,
			Minimum:  props.Minimum,
			Boundary: ring,
		})
	}

	if len(parsed) == 0 {
		panic("contract: service-area.geojson defines no zones")
	}
	return parsed
}

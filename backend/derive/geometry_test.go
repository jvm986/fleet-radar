package derive

import (
	"testing"

	"fleetradar/contract"
)

// The zone geometry is hand authored, so nothing but a test says it is over the right city or
// that the named zones sit where their names claim. These are real Las Vegas landmarks, which
// also makes the file's coordinates readable to somebody who has never seen it drawn.
//
// The gaps between districts matter as much as the districts do. The zones subdivide the service
// area but do not tile it, so a vehicle can be inside the area and in no zone — the case any
// coverage aggregation is likeliest to drop (PRODUCT-SPEC §7.2, ADR-0004 §4.10).
func TestTheZonesAreWhereTheirNamesSay(t *testing.T) {
	for _, tc := range []struct {
		place    string
		position contract.Point
		want     contract.ZoneID
	}{
		{"the Bellagio, on the Strip", contract.Point{-115.1765, 36.1126}, "strip"},
		{"Fremont Street, downtown", contract.Point{-115.1440, 36.1699}, "downtown"},
		{"Boulder Highway, east", contract.Point{-115.0800, 36.1200}, "east-las-vegas"},
		{"Craig Road, north", contract.Point{-115.1200, 36.2200}, "north-las-vegas"},
		{"Summerlin Parkway, west", contract.Point{-115.2700, 36.1700}, "summerlin"},
		{"Harry Reid airport, at the south end of the Strip", contract.Point{-115.1520, 36.0800}, "strip"},
		{"Decatur and Russell, between districts", contract.Point{-115.2080, 36.0830}, contract.ZoneNone},
		{"Los Angeles, well outside the service area", contract.Point{-118.2437, 34.0522}, contract.ZoneNone},
	} {
		t.Run(tc.place, func(t *testing.T) {
			if got := zoneFor(contract.Zones(), tc.position); got != tc.want {
				t.Errorf("zoneFor(%s) = %q, want %q", tc.place, got, tc.want)
			}
		})
	}
}

// A zone reaching outside the area it belongs to is nonsense nothing else would catch: it would
// count vehicles towards coverage for ground the operator is not responsible for, and it would
// draw a boundary crossing the one boundary that is supposed to contain it.
func TestEveryZoneLiesInsideTheServiceArea(t *testing.T) {
	area := contract.ServiceArea()
	for _, zone := range contract.Zones() {
		for _, corner := range zone.Boundary {
			if !contract.Contains(area, corner) {
				t.Errorf("zone %q has a corner at %v outside the service area", zone.ID, corner)
			}
		}
	}
}

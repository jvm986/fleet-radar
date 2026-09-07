package derive

import (
	"testing"

	"fleetradar/contract"
)

// The zone geometry is hand authored, so nothing but a test says it is over the right city or
// that the named zones sit where their names claim. These are real Las Vegas landmarks, which
// also makes the file's coordinates readable to somebody who has never seen it drawn.
//
// The gap around the airport matters as much as the zones do: it is inside the service area and
// in no zone, which is the case any coverage aggregation is likeliest to drop
// (PRODUCT-SPEC §7.2, ADR-0004 §4.10).
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
		{"Harry Reid airport, between zones", contract.Point{-115.1520, 36.0800}, contract.ZoneNone},
		{"Los Angeles, well outside the service area", contract.Point{-118.2437, 34.0522}, contract.ZoneNone},
	} {
		t.Run(tc.place, func(t *testing.T) {
			if got := zoneFor(contract.Zones(), tc.position); got != tc.want {
				t.Errorf("zoneFor(%s) = %q, want %q", tc.place, got, tc.want)
			}
		})
	}
}

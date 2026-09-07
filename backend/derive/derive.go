// Package derive turns one generation of store state into what the operator sees: attention
// conditions, zone membership, per-zone coverage and whole-fleet counts.
//
// Everything here is a pure function of a snapshot, the geometry and a clock. Nothing is
// stored and nothing is cached, so there is no invalidation logic to get wrong — which is the
// point, because the two invalidation bugs PRODUCT-SPEC §7.2 predicted are both bugs of
// caching (ADR-0004 §4.8–§4.12).
//
// It runs in the backend rather than the browser so that a disconnected client simply receives
// nothing further and its flags freeze, instead of every vehicle drifting into staleness
// because one socket failed (ADR-0001 §4.6, PRODUCT-SPEC §7.6).
package derive

import (
	"cmp"
	"slices"
	"time"

	"fleetradar/contract"
	"fleetradar/fleet"
)

// Snapshot derives the whole operator-facing view from one generation of store state. Vehicles,
// coverage, summary and lifecycle come out of one call because they must describe one
// generation: a summary that disagrees with the map is a defect (PRODUCT-SPEC F8).
//
// The slices are always non-nil, so the client is never handed a null where it expects a list.
func Snapshot(vehicles []fleet.Vehicle, zones []contract.Zone, lifecycle contract.Lifecycle, now time.Time) contract.Snapshot {
	snapshot := contract.Snapshot{
		Lifecycle:           lifecycle,
		PublishedAt:         now,
		Vehicles:            make([]contract.Vehicle, 0, len(vehicles)),
		AwaitingFirstReport: make([]contract.AwaitingReport, 0),
		Coverage:            make([]contract.ZoneCoverage, 0, len(zones)),
	}

	summary := contract.Summary{Total: len(vehicles)}
	available := make(map[contract.ZoneID]int, len(zones))

	for _, vehicle := range vehicles {
		if !vehicle.Reporting {
			snapshot.AwaitingFirstReport = append(snapshot.AwaitingFirstReport, contract.AwaitingReport{
				VehicleID: vehicle.ID,
				Label:     vehicle.Label,
			})
			summary.AwaitingFirstReport++
			continue
		}

		switch vehicle.Status {
		case contract.StatusFree:
			summary.Free++
		case contract.StatusEnRoute:
			summary.EnRoute++
		case contract.StatusWithCustomer:
			summary.WithCustomer++
		}

		zone := zoneFor(zones, vehicle.Position)
		// Only a vehicle that could serve a customer now is coverage. One being driven, by a
		// remote driver or by a customer, is committed (PRODUCT-SPEC §2.5).
		if vehicle.Status == contract.StatusFree {
			if zone == contract.ZoneNone {
				summary.AvailableOutsideAnyZone++
			} else {
				available[zone]++
			}
		}

		stale := now.Sub(vehicle.LastObserved) > contract.StaleAfter
		// Below the threshold, not at it: the legend states 20%, so 20% is not flagged
		// (PRODUCT-SPEC F5).
		lowBattery := vehicle.Battery < contract.LowBatteryPercent
		if stale {
			summary.Stale++
		}
		if lowBattery {
			summary.LowBattery++
		}

		reasons := make([]contract.AttentionReason, 0, 2)
		dominant := contract.AttentionNone
		if stale {
			// Staleness subsumes low energy, and not as a priority call: an energy reading we
			// have stopped hearing about is not a fact. The vehicle may be at 0%, or parked and
			// charging (PRODUCT-SPEC §7.1).
			reasons = append(reasons, contract.AttentionStale)
			dominant = contract.AttentionStale
		}
		if lowBattery {
			reasons = append(reasons, contract.AttentionLowBattery)
			if !stale {
				dominant = contract.AttentionLowBattery
			}
		}

		// A route is drawn only for a vehicle under remote control. That one rule covers both
		// consequences of unordered cross-topic delivery: a vehicle that is EN_ROUTE before its
		// route arrives, and a route still held for a vehicle that has stopped being EN_ROUTE
		// (PRODUCT-SPEC F2, ADR-0003 consequences).
		routeID := ""
		if vehicle.Status == contract.StatusEnRoute {
			routeID = vehicle.Route.RouteID
		}

		snapshot.Vehicles = append(snapshot.Vehicles, contract.Vehicle{
			VehicleID:        vehicle.ID,
			Label:            vehicle.Label,
			Position:         vehicle.Position,
			Heading:          vehicle.Heading,
			Status:           vehicle.Status,
			BatteryPercent:   vehicle.Battery,
			SilentForMs:      silentFor(vehicle.LastObserved, now),
			Attention:        dominant,
			AttentionReasons: reasons,
			RouteID:          routeID,
			ZoneID:           zone,
		})
	}

	for _, zone := range zones {
		count := available[zone.ID]
		snapshot.Coverage = append(snapshot.Coverage, contract.ZoneCoverage{
			ZoneID:    zone.ID,
			Name:      zone.Name,
			Available: count,
			Minimum:   zone.Minimum,
			State:     coverageState(count, zone.Minimum),
		})
	}

	snapshot.Summary = summary
	return snapshot
}

// Routes derives the set of route geometries currently on the map. Only remotely-driven
// vehicles have one, which is what keeps a route on screen belonging to a vehicle the operator
// can see (PRODUCT-SPEC F2).
//
// Ordered by route id because the read path resends this set when it changes, and a set whose
// order wandered would look changed every tick (ADR-0005 §5.5).
func Routes(vehicles []fleet.Vehicle) contract.Routes {
	routes := make([]contract.Route, 0)
	for _, vehicle := range vehicles {
		if vehicle.Reporting && vehicle.Status == contract.StatusEnRoute && vehicle.Route.RouteID != "" {
			routes = append(routes, vehicle.Route)
		}
	}

	slices.SortFunc(routes, func(a, b contract.Route) int { return cmp.Compare(a.RouteID, b.RouteID) })
	return contract.Routes{Routes: routes}
}

// silentFor is how long ago the vehicle last spoke, as measured at publication. The client is
// given the age rather than the timestamp so that its figures freeze while it is disconnected
// instead of advancing against its own clock (PRODUCT-SPEC §7.6).
//
// It floors at zero because an observation timestamp ahead of the backend's clock is possible:
// the producer's clock is authoritative, and ingest warns about it rather than correcting it.
// A negative age is not something we can say.
func silentFor(lastObserved, now time.Time) int64 {
	return max(now.Sub(lastObserved).Milliseconds(), 0)
}

// zoneFor tests each zone in turn, every tick, rather than caching the answer. Coverage changes
// on status transitions as well as on movement, so a cache invalidated on position alone would
// be silently wrong the moment a vehicle changed status without moving a metre — exactly the
// trap PRODUCT-SPEC §7.2 recorded. Deriving removes the invalidation entirely (ADR-0004 §4.9).
func zoneFor(zones []contract.Zone, position contract.Point) contract.ZoneID {
	for _, zone := range zones {
		if contract.Contains(zone.Boundary, position) {
			return zone.ID
		}
	}
	// An explicit value rather than an absent one. Vehicles genuinely belong to no zone —
	// between zones, or outside the service area because a customer drove there — and any
	// aggregation that assumed otherwise would silently drop them (ADR-0004 §4.10).
	return contract.ZoneNone
}

// coverageState distinguishes nothing available from merely short, because a zone below target
// serves customers with degraded response while a zone at zero cannot serve them at all, and
// those provoke different escalations (PRODUCT-SPEC §2.5).
func coverageState(available, minimum int) contract.CoverageState {
	switch {
	case available == 0:
		return contract.CoverageNoneAvailable
	case available < minimum:
		return contract.CoverageBelowMinimum
	}
	return contract.CoverageMeeting
}

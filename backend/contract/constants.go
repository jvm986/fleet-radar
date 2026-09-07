package contract

import "time"

// Every line the system draws on the operator's behalf is one constant here, and its value
// is sent to the client rather than duplicated there. A generated copy in the frontend
// could be edited independently and go stale, at which point the legend would lie about
// what the system is actually applying (ADR-0001 §1.8).
const (
	// LowBatteryPercent matches the consumer-EV mental model, so it needs no explaining,
	// and leaves genuine reserve for a teledriver to reach a charger under the vehicle's
	// own power. One band rather than low/critical tiers, because the operator's response
	// to 18% and to 4% is identical (PRODUCT-SPEC §7.1).
	LowBatteryPercent = 20.0

	// ReportingInterval is uniform across the whole fleet, and that uniformity is the
	// point: if parked vehicles reported less often, "two missed reports" would mean
	// something different per vehicle and the legend could not state it (ADR-0003 §3.13).
	ReportingInterval = time.Second

	// StaleAfterMissedReports is expressed in reports rather than seconds so that it stays
	// correct if the cadence changes; a hardcoded duration becomes silently wrong the
	// moment the reporting rate moves. One missed report would strobe the fleet in and out
	// of stale on ordinary jitter (PRODUCT-SPEC §7.1).
	StaleAfterMissedReports = 2

	// StaleAfter is the threshold derivation applies. At 1 Hz this is around two seconds,
	// which is tighter than a real operation on cellular telematics would use — accepted
	// because the simulator controls cadence precisely, and because a reviewer then sees
	// the stale state appear within seconds (ADR-0003 §3.13).
	StaleAfter = ReportingInterval * StaleAfterMissedReports

	// TickInterval sits strictly inside the freshness budget, leaving room for
	// serialisation, transport and render. Because publishing is tick-based, this bounds
	// latency from below and is therefore a product-visible constant (ADR-0005 §5.2).
	TickInterval = 200 * time.Millisecond

	// FreshnessBudget is the guarantee from an event being ingested to the change being
	// visible to a connected operator. Asserted for the ingest-to-publish half; the half
	// past the process boundary is instrumented rather than claimed (ADR-0009 §9.4).
	FreshnessBudget = 250 * time.Millisecond
)

// NewConfig assembles the message every client receives first. It is the single place
// where thresholds become wire values, which is what keeps the operator's legend and the
// backend's logic from coming apart.
func NewConfig() Config {
	return Config{
		TickIntervalMs: int(TickInterval.Milliseconds()),
		Thresholds: Thresholds{
			LowBatteryPercent:       LowBatteryPercent,
			ReportingIntervalMs:     int(ReportingInterval.Milliseconds()),
			StaleAfterMissedReports: StaleAfterMissedReports,
			StaleAfterMs:            int(StaleAfter.Milliseconds()),
		},
		ServiceArea: serviceAreaGeoJSON,
	}
}

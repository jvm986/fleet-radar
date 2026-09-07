package contract

import (
	"encoding/json"
	"time"
)

// This file is the client-facing half of the contract, and the TypeScript the web app
// compiles against is generated from it. Anything added here reaches the browser.
//
// MessageKind names one of the three kinds travelling on the one stream: config once at the start,
// routes on connect and on change, and a snapshot every tick. These are the SSE event names
// (ADR-0005 §5.3).
//
//tsgen:closed
type MessageKind string

const (
	MessageConfig   MessageKind = "config"
	MessageRoutes   MessageKind = "routes"
	MessageSnapshot MessageKind = "snapshot"
)

// Config is sent once, first, on every connection — first so that it is present before
// any snapshot needs it. The client renders the legend from what it receives here and
// holds no copy of its own, so the legend cannot state a threshold the backend is not
// applying (ADR-0001 §1.8, ADR-0005 §5.12).
type Config struct {
	// TickIntervalMs is how often a snapshot is published. The client sizes its
	// disconnection watchdog from it rather than from a local copy (ADR-0005 §5.9).
	TickIntervalMs int `json:"tickIntervalMs"`

	Thresholds Thresholds `json:"thresholds"`

	// ServiceArea is the checked-in GeoJSON verbatim: the service area boundary and the
	// named zones, each zone carrying its name and its minimum in its properties. The
	// client draws it; nothing else reads it.
	ServiceArea json.RawMessage `json:"serviceArea"`
}

// Thresholds are the lines the system draws on the operator's behalf. They are sent
// because the legend has to state them, so that the operator knows what is being decided
// for them without going looking (PRODUCT-SPEC F8).
type Thresholds struct {
	LowBatteryPercent float64 `json:"lowBatteryPercent"`

	ReportingIntervalMs     int `json:"reportingIntervalMs"`
	StaleAfterMissedReports int `json:"staleAfterMissedReports"`
	// StaleAfterMs is the two figures above multiplied out. Sent rather than left to the
	// client, so that no arithmetic on policy happens there.
	StaleAfterMs int `json:"staleAfterMs"`
}

// Routes is the complete current set of routes, sent on connect and whenever the set
// changes. Geometry is large and slow-changing, so repeating it in a 200 ms snapshot would
// let static polylines dominate the payload; snapshots reference a route by id instead
// (ADR-0005 §5.5).
//
// The whole set is sent rather than a diff, which keeps the client's one piece of cached
// state a straight replacement — nothing to merge, and nothing to remove. Every route is drawn
// faint with the selected vehicle's emphasised, and a route the client cannot match to a drawn
// vehicle is simply not drawn: degraded, never wrong (PRODUCT-SPEC F2).
type Routes struct {
	Routes []Route `json:"routes"`
}

// Snapshot is one generation of operator-facing state, published every tick whether or not
// anything changed. The unconditional tick is what makes silence diagnostic: because a
// snapshot always arrives, its absence is unambiguous and needs no separate heartbeat
// (ADR-0005 §5.1, §5.2, §5.7).
//
// Vehicles, coverage, summary and lifecycle travel together because they must describe the
// same generation of state. Delivered separately, coverage computed at one tick could
// reach the client alongside vehicles from the next, and a summary that disagrees with the
// map is a defect (PRODUCT-SPEC F8).
type Snapshot struct {
	Lifecycle Lifecycle `json:"lifecycle"`

	// PublishedAt is carried for the freshness instrumentation the client logs in
	// development. The ingest-to-publish half of the budget is asserted in tests; this
	// half is measured rather than claimed (ADR-0006 §6.10, ADR-0009 §9.4).
	PublishedAt time.Time `json:"publishedAt"`

	Vehicles            []Vehicle        `json:"vehicles"`
	AwaitingFirstReport []AwaitingReport `json:"awaitingFirstReport"`
	Coverage            []ZoneCoverage   `json:"coverage"`
	Summary             Summary          `json:"summary"`
}

// Lifecycle is the backend's completeness claim, and it travels inside the snapshot rather
// than alongside it so there is no window in which the client holds data but not the claim
// about that data (ADR-0005 §5.12).
//
//tsgen:closed
type Lifecycle string

const (
	// LifecycleStarting means registration replay has not finished, so the fleet is still
	// filling. The operator must be told that rather than shown a partial fleet as though
	// it were complete (PRODUCT-SPEC F6, ADR-0004 §4.11).
	LifecycleStarting Lifecycle = "STARTING"
	LifecycleReady    Lifecycle = "READY"
)

// Vehicle is a vehicle the operator can be shown on the map: one that has reported its
// position, heading, battery and status at least once. Every value is last known rather
// than current, which is what SilentForMs qualifies.
type Vehicle struct {
	VehicleID string `json:"vehicleId"`
	Label     string `json:"label"`

	Position       Point         `json:"position"`
	Heading        float64       `json:"heading"`
	Status         VehicleStatus `json:"status"`
	BatteryPercent float64       `json:"batteryPercent"`

	// SilentForMs is how long before PublishedAt the newest of this vehicle's
	// observations was made. The age is sent rather than the timestamp so that a
	// disconnected client's figures freeze instead of advancing against its own clock:
	// blaming a hundred healthy vehicles for one failed connection is a false claim, not a
	// conservative one (PRODUCT-SPEC §7.6).
	SilentForMs int64 `json:"silentForMs"`

	// Attention is the one condition the map shows, and it is AttentionNone for a vehicle
	// that warrants none. Staleness subsumes low energy — not as a priority call but a
	// logical one, since an energy reading we have stopped hearing about is not a fact
	// (PRODUCT-SPEC §7.1).
	Attention AttentionReason `json:"attention"`
	// AttentionReasons is every condition that applies. Where both do, the panel states
	// both even though the map shows only the dominant one (PRODUCT-SPEC F3).
	AttentionReasons []AttentionReason `json:"attentionReasons"`

	// RouteID references a route in the routes message, and is empty when no route is to
	// be drawn. A vehicle can be EN_ROUTE with no route yet, because arrival order across
	// topics is not guaranteed; drawing a route only for an EN_ROUTE vehicle resolves that
	// and the converse case together (ADR-0003 consequences, PRODUCT-SPEC F2).
	RouteID string `json:"routeId"`

	// ZoneID is ZoneNone for a vehicle in no zone — outside the service area, or in a gap
	// between zones. An explicit value rather than an absent one, because vehicles
	// silently dropped from a zone total is the likeliest correctness bug in the design
	// (PRODUCT-SPEC §7.2, ADR-0004 §4.10).
	ZoneID ZoneID `json:"zoneId"`
}

// AwaitingReport is a vehicle known from its registration that has not yet reported enough
// to be drawn. It cannot be a marker, because there is nowhere to put one.
//
// It is not stale: stale means we had it and lost it, awaiting a first report means we have
// never had it, and the two call for different responses (PRODUCT-SPEC F6). Carried so the
// summary can account for every registered vehicle, and so label search can still find one
// (ADR-0004 §4.8).
type AwaitingReport struct {
	VehicleID string `json:"vehicleId"`
	Label     string `json:"label"`
}

// AttentionReason is the closed list of conditions that make a vehicle warrant attention.
// Both are derived from state the system already holds; neither is reported to us as a
// problem. The two are kept distinguishable because they imply different work: low energy
// is scheduling, silence is a possible fault and the field-agent case (PRODUCT-SPEC §2.4).
//
//tsgen:closed
type AttentionReason string

const (
	AttentionNone       AttentionReason = "NONE"
	AttentionLowBattery AttentionReason = "LOW_BATTERY"
	AttentionStale      AttentionReason = "STALE"
)

// ZoneID identifies one named zone within the service area. Zone ids come from the checked-in
// geometry rather than from this file, so it is deliberately not a closed set — ZoneNone is the one
// value the code itself decides.
type ZoneID string

// ZoneNone is the zone of a vehicle that is in none of them. Vehicles genuinely can be:
// a customer driving one may leave the service area entirely, and the zones do not tile
// the area (PRODUCT-SPEC §2.5).
const ZoneNone ZoneID = "NO_ZONE"

// ZoneCoverage is whether one zone currently has enough vehicles that could serve a
// customer now. Only available vehicles count — a vehicle being driven, by a remote driver
// or by a customer, is committed — so coverage changes on status transitions as well as on
// movement (PRODUCT-SPEC §2.5, §7.2).
type ZoneCoverage struct {
	ZoneID ZoneID `json:"zoneId"`
	Name   string `json:"name"`

	Available int `json:"available"`
	// Minimum is sent so the expectation is discoverable: the operator can see what line
	// is being drawn on their behalf (PRODUCT-SPEC F7).
	Minimum int           `json:"minimum"`
	State   CoverageState `json:"state"`
}

// CoverageState distinguishes zero available from merely short, because a zone below
// target serves customers with degraded response while a zone at zero cannot serve them at
// all, and those provoke different escalations (PRODUCT-SPEC §2.5, §7.2).
//
//tsgen:closed
type CoverageState string

const (
	CoverageMeeting       CoverageState = "MEETING"
	CoverageBelowMinimum  CoverageState = "BELOW_MINIMUM"
	CoverageNoneAvailable CoverageState = "NONE_AVAILABLE"
)

// Summary describes the whole fleet and never the filtered subset. Selecting a figure
// applies the corresponding filter, and the figure must not change when it does — a figure
// that tracked its own filter would be a defect (PRODUCT-SPEC F8).
type Summary struct {
	// Total is every registered vehicle. It is the sum of the three status counts and
	// AwaitingFirstReport, and Total minus AwaitingFirstReport is the number of markers on
	// an unfiltered map. That reconciliation is what makes the summary trustworthy, so a
	// figure disagreeing with the map is treated as a defect rather than a discrepancy.
	Total int `json:"total"`

	Free         int `json:"free"`
	EnRoute      int `json:"enRoute"`
	WithCustomer int `json:"withCustomer"`

	// LowBattery and Stale count conditions rather than dominant conditions, so a vehicle
	// that is both is counted in both. That is what makes each figure agree with the
	// filter it applies, since filters select on the condition (PRODUCT-SPEC F4, F8).
	LowBattery int `json:"lowBattery"`
	Stale      int `json:"stale"`

	AwaitingFirstReport int `json:"awaitingFirstReport"`

	// AvailableOutsideAnyZone completes the coverage figures: it plus every zone's
	// Available equals Free. Without it, available vehicles in no zone would be absent
	// from every coverage total the operator could cross-check (PRODUCT-SPEC F7, §7.2).
	AvailableOutsideAnyZone int `json:"availableOutsideAnyZone"`
}

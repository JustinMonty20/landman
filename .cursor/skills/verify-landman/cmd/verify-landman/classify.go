package main

import (
	"strings"
	"time"
)

const (
	fastMetadataLimit = 5 * time.Second
	slowPageLimit     = 15 * time.Second
	commitLine        = "Imported parcels,"
)

type verdict string

const (
	verdictPass       verdict = "pass"
	verdictFail       verdict = "fail"
	verdictCountySlow verdict = "inconclusive: county-source-slow"
)

type tier string

const (
	tierMet    tier = "met"
	tierMissed tier = "missed"
	tierNotRun tier = "not-run"
)

// driveOutcome is the classify record. Sales is nil when the parcel
// transaction did not commit. The JSON form has no boolean field.
type driveOutcome struct {
	Verdict     verdict    `json:"verdict"`
	SeededSales tier       `json:"seededSales"`
	FullImport  tier       `json:"fullImport"`
	Sales       []saleSlot `json:"sales"`
	Problems    []string   `json:"-"`
}

func (o driveOutcome) code() int {
	switch o.Verdict {
	case verdictPass:
		return 0
	case verdictCountySlow:
		return 2
	default:
		return 1
	}
}

// timedCall is one probe sample. Error "timeout" means the client gave up.
type timedCall struct {
	HTTPStatus int    `json:"httpStatus"`
	DurationMS int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

type pageTimings struct {
	ParcelMetadata timedCall `json:"parcelMetadata"`
	SoilMetadata   timedCall `json:"soilMetadata"`
	ParcelPage     timedCall `json:"parcelPage"`
	SoilPage       timedCall `json:"soilPage"`
}

type postImport struct {
	CountyParcels         int
	SeedParcels           int
	SoilPolygons          int
	SeedSoilPolygons      int
	Overlaps              int
	SeedOverlaps          int
	ParcelJobSuccesses    int
	SoilJobSuccesses      int
	ScreeningJobSuccesses int
	ScreeningHeads        int
	Assessments           int
}

func (p postImport) matches() bool {
	return p.CountyParcels > p.SeedParcels &&
		p.SoilPolygons > p.SeedSoilPolygons &&
		p.Overlaps > p.SeedOverlaps &&
		p.ParcelJobSuccesses >= 1 &&
		p.SoilJobSuccesses >= 1 &&
		p.ScreeningJobSuccesses >= 1 &&
		p.ScreeningHeads == 1 &&
		p.Assessments == p.CountyParcels
}

type classifyInput struct {
	stdout      string
	stderr      string
	processExit int
	probe       pageTimings
	salesRead   bool
	expected    []saleSlot
	actual      []saleSlot
	post        *postImport
}

func classify(in classifyInput) driveOutcome {
	logText := in.stdout + "\n" + in.stderr
	didCommit := strings.Contains(in.stdout, commitLine) || strings.Contains(in.stderr, commitLine)
	deadline := deadlineText(logText)
	slow := countySourceSlow(in.probe, logText)

	if !didCommit {
		out := driveOutcome{SeededSales: tierNotRun, FullImport: tierMissed}
		if deadline && slow {
			out.Verdict = verdictCountySlow
			return out
		}
		out.Verdict = verdictFail
		return out
	}

	out := driveOutcome{FullImport: tierMissed}
	if !in.salesRead {
		out.Verdict = verdictFail
		out.SeededSales = tierMissed
		out.Problems = []string{"sales not read"}
		return out
	}
	out.Sales = in.actual
	if out.Sales == nil {
		out.Sales = []saleSlot{}
	}
	out.Problems = compareSlots(in.expected, out.Sales)
	if len(out.Problems) > 0 {
		out.Verdict = verdictFail
		out.SeededSales = tierMissed
		if in.processExit == 0 && in.post != nil && in.post.matches() {
			out.FullImport = tierMet
		}
		return out
	}
	out.SeededSales = tierMet
	if in.processExit != 0 {
		if deadline && slow {
			out.Verdict = verdictCountySlow
			return out
		}
		out.Verdict = verdictFail
		return out
	}
	if in.post != nil && in.post.matches() {
		out.Verdict = verdictPass
		out.FullImport = tierMet
		return out
	}
	out.Verdict = verdictFail
	return out
}

func deadlineText(logText string) bool {
	return strings.Contains(logText, "context deadline exceeded") ||
		strings.Contains(logText, "rate: Wait(n=1) would exceed context deadline")
}

func countySourceSlow(probe pageTimings, logText string) bool {
	gap := laterBatchGapSlow(logText)
	if metaFast(probe.ParcelMetadata) && (callSlow(probe.ParcelPage) || gap) {
		return true
	}
	return metaFast(probe.SoilMetadata) && callSlow(probe.SoilPage)
}

func metaFast(call timedCall) bool {
	return call.Error == "" && call.HTTPStatus == 200 && call.DurationMS <= fastMetadataLimit.Milliseconds()
}

func callSlow(call timedCall) bool {
	return call.Error == "timeout" || call.DurationMS >= slowPageLimit.Milliseconds()
}

package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	batchCompletePattern = regexp.MustCompile(`(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?) \[union_county_gis\] Batch (\d+)/\d+ complete:`)
	fetchAnchorPattern   = regexp.MustCompile(`(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?) \[union_county_gis\] (?:Total parcels to fetch:|Batch size:)`)
)

// laterBatchGapSlow reports a parcel batch gap of at least 15 seconds.
// The gap that ends at batch 1 includes the silent soil fetch, so it does not count.
func laterBatchGapSlow(logText string) bool {
	var prev time.Time
	var havePrev bool
	for _, line := range strings.Split(logText, "\n") {
		if when, batch, ok := parseBatchComplete(line); ok {
			if havePrev && batch != 1 && when.Sub(prev) >= slowPageLimit {
				return true
			}
			prev = when
			havePrev = true
			continue
		}
		if when, ok := parseFetchAnchor(line); ok && !havePrev {
			prev = when
			havePrev = true
		}
	}
	return false
}

func parseBatchComplete(line string) (time.Time, int, bool) {
	match := batchCompletePattern.FindStringSubmatch(line)
	if match == nil {
		return time.Time{}, 0, false
	}
	when, err := parseLogTime(match[1])
	if err != nil {
		return time.Time{}, 0, false
	}
	batch, err := strconv.Atoi(match[2])
	if err != nil {
		return time.Time{}, 0, false
	}
	return when, batch, true
}

func parseFetchAnchor(line string) (time.Time, bool) {
	match := fetchAnchorPattern.FindStringSubmatch(line)
	if match == nil {
		return time.Time{}, false
	}
	when, err := parseLogTime(match[1])
	return when, err == nil
}

func parseLogTime(text string) (time.Time, error) {
	if when, err := time.Parse("2006/01/02 15:04:05.000000", text); err == nil {
		return when, nil
	}
	return time.Parse("2006/01/02 15:04:05", text)
}

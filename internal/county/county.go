// Package county names the county ids stored on parcel rows.
//
// Parse is the boundary for untyped text. Valid reports whether a converted
// ID is one of All. A caller that holds a value returned by Parse does not
// call Valid again.
package county

import (
	"errors"
	"fmt"
)

// ID is a county identifier. Text from outside this package stays a string
// until Parse accepts it.
type ID string

// Union is the Union County id stored in parcels.county_id.
const Union ID = "union"

// ErrUnknown is the error Parse wraps when raw text is not a value of All.
var ErrUnknown = errors.New("unknown county id")

// Parse returns the county id equal to raw, compared byte for byte with All.
// Any other string returns the zero ID and an error wrapping ErrUnknown.
func Parse(raw string) (ID, error) {
	id := ID(raw)
	if id.Valid() {
		return id, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnknown, raw)
}

// Valid reports whether id equals a value of All.
// Use it for an ID built by conversion. A value returned by Parse is already known.
func (id ID) Valid() bool {
	for _, known := range All() {
		if id == known {
			return true
		}
	}
	return false
}

// String returns the underlying text, including text that fails Valid.
func (id ID) String() string {
	return string(id)
}

// All returns the known county ids in parcels_county_id_check order.
// Each call returns a new slice, so a caller that appends does not change
// the next call.
func All() []ID {
	return []ID{Union}
}

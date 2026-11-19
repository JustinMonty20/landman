package union

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// AssessmentObjectID reads the same decoded identity that publication uses, rejecting
// ambiguous attribute envelopes and identity members before range membership checks.
func AssessmentObjectID(feature []byte) (int64, error) {
	d := json.NewDecoder(bytes.NewReader(feature))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return 0, errors.New("invalid assessment feature")
	}
	var attrs json.RawMessage
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return 0, err
		}
		var value json.RawMessage
		if err = d.Decode(&value); err != nil {
			return 0, err
		}
		if strings.EqualFold(key.(string), "attributes") {
			if attrs != nil {
				return 0, errors.New("duplicate assessment attributes")
			}
			attrs = value
		}
	}
	if _, err = d.Token(); err != nil || len(attrs) == 0 {
		return 0, errors.New("missing assessment attributes")
	}
	a := json.NewDecoder(bytes.NewReader(attrs))
	start, err = a.Token()
	if err != nil || start != json.Delim('{') {
		return 0, errors.New("invalid assessment attributes")
	}
	var raw json.RawMessage
	for a.More() {
		key, err := a.Token()
		if err != nil {
			return 0, err
		}
		var value json.RawMessage
		if err = a.Decode(&value); err != nil {
			return 0, err
		}
		if key == "OBJECTID" {
			if raw != nil {
				return 0, errors.New("duplicate assessment OBJECTID")
			}
			raw = value
		}
	}
	if _, err = a.Token(); err != nil || len(raw) == 0 {
		return 0, errors.New("missing assessment OBJECTID")
	}
	var n json.Number
	if err = json.Unmarshal(raw, &n); err != nil {
		return 0, errors.New("invalid assessment OBJECTID")
	}
	id, err := strconv.ParseInt(n.String(), 10, 64)
	if err != nil || id < 0 {
		return 0, errors.New("invalid assessment OBJECTID")
	}
	return id, nil
}

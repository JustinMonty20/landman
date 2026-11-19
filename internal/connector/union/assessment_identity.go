package union

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

// DecodeAssessmentPage rejects ambiguous feature arrays before capture or replay selects rows.
func DecodeAssessmentPage(body []byte) ([]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(body))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return nil, errors.New("invalid assessment page")
	}
	var features []json.RawMessage
	seen := false
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		if key == "features" {
			if seen {
				return nil, errors.New("duplicate assessment features")
			}
			seen = true
			if err := json.Unmarshal(value, &features); err != nil || features == nil {
				return nil, errors.New("invalid assessment features")
			}
		}
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF || !seen {
		return nil, errors.New("invalid assessment page ending or missing features")
	}
	return features, nil
}

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
	id, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || id < 0 || string(raw) != strconv.FormatInt(id, 10) {
		return 0, errors.New("invalid assessment OBJECTID")
	}
	return id, nil
}

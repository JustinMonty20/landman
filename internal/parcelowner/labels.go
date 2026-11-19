// Package parcelowner classifies normalized current-owner names and mailing states.
package parcelowner

import (
	"strings"
	"unicode"

	"github.com/JustinMonty20/landman/internal/models"
)

type MailingStatus string

const (
	MailingYes     MailingStatus = "yes"
	MailingNo      MailingStatus = "no"
	MailingUnknown MailingStatus = "unknown"
)

type NameStatus string

const (
	NameMatched     NameStatus = "matched"
	NameNotDetected NameStatus = "not_detected"
	NameUnknown     NameStatus = "unknown"
)

// Labels describes name-based indicators, not verified legal status or owner residence.
type Labels struct {
	OutOfStateMailing      MailingStatus `json:"out_of_state_mailing"`
	EntityNameIndicator    NameStatus    `json:"entity_name_indicator"`
	CorporateNameIndicator NameStatus    `json:"corporate_name_indicator"`
	MailingState           *string       `json:"mailing_state"`
	MatchedSuffix          *string       `json:"matched_suffix"`
	MatchedOwnerLine       *int          `json:"matched_owner_line"`
}

var states = strings.Fields("AL AK AZ AR CA CO CT DE FL GA HI ID IL IN IA KS KY LA ME MD MA MI MN MS MO MT NE NV NH NJ NM NY NC ND OH OK OR PA RI SC SD TN TX UT VT VA WA WV WI WY DC AS GU MP PR VI")
var corporate = map[string]bool{"INC": true, "INCORPORATED": true, "CORP": true, "CORPORATION": true}
var otherEntity = map[string]bool{"LLC": true, "LP": true, "LLP": true, "LTD": true, "LIMITED": true, "PLC": true, "PC": true}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}
func suffix(name string) string {
	name = strings.TrimRightFunc(strings.TrimSpace(name), func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSpace(r) })
	parts := strings.Fields(name)
	if len(parts) < 2 {
		return ""
	}
	last := strings.ToUpper(strings.ReplaceAll(parts[len(parts)-1], ".", ""))
	if corporate[last] || otherEntity[last] {
		return last
	}
	return ""
}

// Classify considers only complete current name lines and the explicit mailing state.
func Classify(owner models.ParcelOwner, mailing models.ParcelAddress) Labels {
	result := Labels{OutOfStateMailing: MailingUnknown, EntityNameIndicator: NameUnknown, CorporateNameIndicator: NameUnknown}
	state := strings.ToUpper(value(mailing.State))
	for _, valid := range states {
		if state == valid {
			result.MailingState = &state
			result.OutOfStateMailing = MailingYes
			if state == "NC" {
				result.OutOfStateMailing = MailingNo
			}
			break
		}
	}
	names := []string{value(owner.CurrentName1), value(owner.CurrentName2)}
	for i, name := range names {
		if name == "" {
			continue
		}
		if result.EntityNameIndicator == NameUnknown {
			result.EntityNameIndicator = NameNotDetected
			result.CorporateNameIndicator = NameNotDetected
		}
		found := suffix(name)
		if found == "" {
			continue
		}
		if result.EntityNameIndicator == NameMatched && !corporate[found] {
			continue
		}
		result.EntityNameIndicator = NameMatched
		result.MatchedSuffix = &found
		line := i + 1
		result.MatchedOwnerLine = &line
		if corporate[found] {
			result.CorporateNameIndicator = NameMatched
			break
		}
	}
	return result
}

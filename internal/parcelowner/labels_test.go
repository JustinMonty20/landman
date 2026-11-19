package parcelowner_test

import (
	"testing"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelowner"
)

func ptr(s string) *string { return &s }
func TestClassify(t *testing.T) {
	cases := []struct {
		name, state, owner1, owner2 string
		mailing                     parcelowner.MailingStatus
		entity, corp                parcelowner.NameStatus
		suffix                      string
		line                        int
	}{
		{"SC LLC", "SC", "ACME LLC", "", "yes", "matched", "not_detected", "LLC", 1},
		{"trim NC", " nc ", "PERSON", "", "no", "not_detected", "not_detected", "", 0},
		{"blank", "", "", "", "unknown", "unknown", "unknown", "", 0},
		{"garbage", "XX", "TRUST", "", "unknown", "not_detected", "not_detected", "", 0},
		{"DC punctuation", "DC", "ACME L.L.C.", "", "yes", "matched", "not_detected", "LLC", 1},
		{"territory corporation", "PR", "ACME INC.", "", "yes", "matched", "matched", "INC", 1},
		{"military", "AE", "ACME CORPORATION", "", "unknown", "matched", "matched", "CORPORATION", 1},
		{"foreign", "ON", "ACME CORP", "", "unknown", "matched", "matched", "CORP", 1},
		{"company", "NC", "ACME COMPANY", "", "no", "not_detected", "not_detected", "", 0},
		{"co", "NC", "ACME CO", "", "no", "not_detected", "not_detected", "", 0},
		{"substring", "NC", "LINCOLN", "", "no", "not_detected", "not_detected", "", 0},
		{"split", "NC", "ACME", "LLC", "no", "not_detected", "not_detected", "", 0},
		{"corporation wins", "NC", "ACME LLC", "OTHER CORP", "no", "matched", "matched", "CORP", 2},
		{"LP", "NC", "ACME LP", "", "no", "matched", "not_detected", "LP", 1},
		{"trust", "NC", "TRUST", "", "no", "not_detected", "not_detected", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parcelowner.Classify(models.ParcelOwner{CurrentName1: ptr(tc.owner1), CurrentName2: ptr(tc.owner2)}, models.ParcelAddress{State: ptr(tc.state)})
			if got.OutOfStateMailing != tc.mailing || got.EntityNameIndicator != tc.entity || got.CorporateNameIndicator != tc.corp {
				t.Fatalf("statuses: %+v", got)
			}
			if tc.suffix == "" {
				if got.MatchedSuffix != nil || got.MatchedOwnerLine != nil {
					t.Fatalf("unexpected match evidence: %+v", got)
				}
			} else if got.MatchedSuffix == nil || *got.MatchedSuffix != tc.suffix || got.MatchedOwnerLine == nil || *got.MatchedOwnerLine != tc.line {
				t.Fatalf("evidence: %+v", got)
			}
			if tc.mailing == "unknown" && got.MailingState != nil {
				t.Fatalf("unknown state evidence: %+v", got)
			}
			if got.CorporateNameIndicator == "matched" && got.EntityNameIndicator != "matched" {
				t.Fatalf("corporate without entity: %+v", got)
			}
		})
	}
}

package main

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

// saleSlot is one kept s1, s2, or s3 row. Empty strings stand for SQL NULL.
// Sequence stays 1, 2, or 3.
type saleSlot struct {
	SourceParcelID string `json:"sourceParcelId"`
	Sequence       int    `json:"sequence"`
	SaleDate       string `json:"saleDate,omitempty"`
	Amount         string `json:"amount,omitempty"`
	DeedBook       string `json:"deedBook,omitempty"`
	DeedPage       string `json:"deedPage,omitempty"`
}

type slotKey struct {
	id  string
	seq int
}

func expectedSaleSlots(sourceParcelID string, attrs map[string]any) []saleSlot {
	prefixes := []string{"s1", "s2", "s3"}
	slots := make([]saleSlot, 0, len(prefixes))
	for i, prefix := range prefixes {
		slot, ok := slotFromAttrs(sourceParcelID, i+1, prefix, attrs)
		if ok {
			slots = append(slots, slot)
		}
	}
	return slots
}

func slotFromAttrs(sourceParcelID string, sequence int, prefix string, attrs map[string]any) (saleSlot, bool) {
	if attrs == nil {
		return saleSlot{}, false
	}
	date, hasDate := arcGISDay(attrs[prefix+"_SALEDATE"])
	amount, hasAmount := arcGISAmount(attrs[prefix+"_SALESAMT"])
	book := attrText(attrs[prefix+"_DEED_BOOK"])
	page := attrText(attrs[prefix+"_DEED_PAGE"])
	deedType := attrText(attrs[prefix+"_DEEDTYPE"])
	quality := attrText(attrs[prefix+"_DEEDQUAL_CODEDESC"])
	grantor := attrText(attrs[prefix+"_grantor"])
	grantor2 := attrText(attrs[prefix+"_grantor2"])
	nonzeroAmount := hasAmount && amount.Sign() != 0
	hasDeed := book != "" || page != "" || deedType != "" || quality != "" || grantor != "" || grantor2 != ""
	if !hasDate && !nonzeroAmount && !hasDeed {
		return saleSlot{}, false
	}
	slot := saleSlot{SourceParcelID: sourceParcelID, Sequence: sequence, DeedBook: book, DeedPage: page}
	if hasDate {
		slot.SaleDate = date
	}
	if hasAmount {
		slot.Amount = amount.FloatString(2)
	}
	return slot, true
}

func arcGISDay(value any) (string, bool) {
	millis, ok := wholeInt(value)
	if !ok || millis == 0 {
		return "", false
	}
	return time.UnixMilli(millis).UTC().Format("2006-01-02"), true
}

func arcGISAmount(value any) (*big.Rat, bool) {
	switch v := value.(type) {
	case nil:
		return nil, false
	case *big.Rat:
		if v == nil {
			return nil, false
		}
		return new(big.Rat).Set(v), true
	case int:
		return big.NewRat(int64(v), 1), true
	case int64:
		return big.NewRat(v, 1), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, false
		}
		return ratFromDecimal(strconv.FormatFloat(v, 'f', 2, 64))
	case float32:
		return arcGISAmount(float64(v))
	case string:
		return ratFromDecimal(v)
	default:
		return ratFromDecimal(fmt.Sprint(v))
	}
}

func ratFromDecimal(text string) (*big.Rat, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, false
	}
	rat, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, false
	}
	return rat, true
}

func wholeInt(value any) (int64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) {
			return 0, false
		}
		return int64(v), true
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func attrText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		if v == math.Trunc(v) && !math.IsNaN(v) && !math.IsInf(v, 0) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strings.TrimSpace(strconv.FormatFloat(v, 'f', -1, 64))
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func compareSlots(expected, actual []saleSlot) []string {
	want := make(map[slotKey]saleSlot, len(expected))
	for _, slot := range expected {
		want[slotKey{slot.SourceParcelID, slot.Sequence}] = slot
	}
	seen := make(map[slotKey]saleSlot, len(actual))
	var problems []string
	for _, slot := range actual {
		key := slotKey{slot.SourceParcelID, slot.Sequence}
		if _, ok := want[key]; !ok {
			problems = append(problems, fmt.Sprintf("presence %s sequence %d", slot.SourceParcelID, slot.Sequence))
			continue
		}
		if _, dup := seen[key]; dup {
			problems = append(problems, fmt.Sprintf("presence %s sequence %d", slot.SourceParcelID, slot.Sequence))
			continue
		}
		seen[key] = slot
	}
	for key, slot := range want {
		got, ok := seen[key]
		if !ok {
			problems = append(problems, fmt.Sprintf("missing %s sequence %d", key.id, key.seq))
			continue
		}
		if !slotFieldsEqual(slot, got) {
			problems = append(problems, fmt.Sprintf("mismatch %s sequence %d", key.id, key.seq))
		}
	}
	sort.Strings(problems)
	return problems
}

func slotFieldsEqual(want, got saleSlot) bool {
	if want.SaleDate != got.SaleDate {
		return false
	}
	if !decimalsEqual(want.Amount, got.Amount) {
		return false
	}
	return want.DeedBook == got.DeedBook && want.DeedPage == got.DeedPage
}

func decimalsEqual(want, got string) bool {
	want = strings.TrimSpace(want)
	got = strings.TrimSpace(got)
	if want == "" || got == "" {
		return want == got
	}
	left, okLeft := ratFromDecimal(want)
	right, okRight := ratFromDecimal(got)
	if !okLeft || !okRight {
		return false
	}
	return left.Cmp(right) == 0
}

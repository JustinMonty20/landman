package spatialist

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// SpatialistFlatRecord is a flattened view of the Spatialist parcel payload.
// It keeps the top-level fields and flattens selected sections into single objects or arrays.
type SpatialistFlatRecord struct {
	ParcelID    string                 `json:"parcelId"`
	Valuation   SpatialistValuation    `json:"valuation"`
	Sales       []SpatialistSale       `json:"sales"`
	Permits     []SpatialistPermit     `json:"permits"`
	Assessments []SpatialistAssessment `json:"assessments"`
}

// SpatialistFlatEnvelope namespaces the flattened record under the source name.
type SpatialistFlatEnvelope struct {
	UCRealPropertySearch *SpatialistFlatRecord `json:"uc_real_property_search"`
}

// SpatialistValuation contains parsed valuation data plus original strings.
type SpatialistValuation struct {
	FMVLand    int64  `json:"FMV_LAND"`
	OGFMVLand  string `json:"og_FMV_LAND"`
	FMVImprv   int64  `json:"FMV_IMPRV"`
	OGFMVImprv string `json:"og_FMV_IMPRV"`
	FMVTotal   int64  `json:"FMV_TOTAL"`
	OGFMVTotal string `json:"og_FMV_TOTAL"`
	ASDLand    int64  `json:"ASD_Land"`
	OGASDLand  string `json:"og_ASD_Land"`
	ASDBLDG    int64  `json:"ASD_BLDG"`
	OGASDBLDG  string `json:"og_ASD_BLDG"`
	ASDTotal   int64  `json:"ASD_TOTAL"`
	OGASDTotal string `json:"og_ASD_TOTAL"`
}

// SpatialistSale contains flattened sale history data.
type SpatialistSale struct {
	DocumentNumber      int64  `json:"document_number"`
	Book                string `json:"book"`
	Page                string `json:"page"`
	SaleType            string `json:"sale_type"`
	DateOfSale          string `json:"date_of_sale"`
	Order0              string `json:"order_0"`
	GrossSellingPrice   int64  `json:"gross_selling_price"`
	OGGrossSellingPrice string `json:"og_gross_selling_price"`
	Grantor             string `json:"grantor"`
	Grantee             string `json:"grantee"`
}

// SpatialistAssessment contains flattened assessment history data.
type SpatialistAssessment struct {
	AssessDate      string `json:"assess_date"`
	Order0          string `json:"order_0"`
	ParcelYear      int64  `json:"parcel_year"`
	Order1          int64  `json:"order_1"`
	ChangeReason    string `json:"change_reason"`
	TotalAssessed   int64  `json:"TotalAssessed"`
	OGTotalAssessed string `json:"og_TotalAssessed"`
}

// SpatialistPermit contains flattened permit data.
type SpatialistPermit struct {
	PermitNumber           int64  `json:"PermitNumber"`
	IssuedDate             string `json:"IssuedDate"`
	ReportCode             string `json:"ReportCode"`
	PermitTemplate         string `json:"PermitTemplate"`
	ContractorOrganization string `json:"ContractorOrganization"`
	PermitDescription      string `json:"PermitDescription"`
}

type spatialistRawRecord struct {
	ID       string              `json:"id"`
	NBHD     json.Number         `json:"nbhd"`
	CTX      string              `json:"ctx"`
	CTY      string              `json:"cty"`
	Active   bool                `json:"active"`
	ImageID  string              `json:"imageid"`
	SketchID string              `json:"sketchid"`
	Parcel   spatialistRawParcel `json:"parcel"`
}

type spatialistRawParcel struct {
	Header          map[string]interface{} `json:"header"`
	Bounds          string                 `json:"bounds"`
	MapIcon         interface{}            `json:"mapicon"`
	MapIconSelected interface{}            `json:"mapiconselected"`
}

type rowSubdataWrapper struct {
	Row     map[string]interface{} `json:"row"`
	Subdata map[string]interface{} `json:"subdata"`
}

// FlattenSpatialistRecord flattens the nested Spatialist "sections" array into
// named objects/slices for easier consumption.
func FlattenSpatialistRecord(raw map[string]interface{}) (*SpatialistFlatEnvelope, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw record cannot be nil")
	}

	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal spatialist record: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var top spatialistRawRecord
	if err := decoder.Decode(&top); err != nil {
		return nil, fmt.Errorf("decode spatialist record: %w", err)
	}

	valuationObj, err := extractObject(payload, "parcel.sections.0.1.0")
	if err != nil {
		return nil, err
	}

	salesRaw, err := extractMergedArray(payload, "parcel.sections.3.0")
	if err != nil {
		return nil, err
	}

	assessmentsRaw, err := extractArrayObjects(payload, "parcel.sections.4.0")
	if err != nil {
		return nil, err
	}

	permitsRaw, err := extractMergedArray(payload, "parcel.sections.5.0")
	if err != nil {
		return nil, err
	}

	valuation := parseValuation(valuationObj)
	sales, err := parseSales(salesRaw)
	if err != nil {
		return nil, err
	}

	assessments, err := parseAssessments(assessmentsRaw)
	if err != nil {
		return nil, err
	}

	permits, err := parsePermits(permitsRaw)
	if err != nil {
		return nil, err
	}

	parcelID := top.ID
	if parcelID == "" {
		if pid, ok := top.Parcel.Header["PID"].(string); ok {
			parcelID = pid
		}
	}

	record := &SpatialistFlatRecord{
		ParcelID:    parcelID,
		Valuation:   valuation,
		Sales:       sales,
		Permits:     permits,
		Assessments: assessments,
	}

	return &SpatialistFlatEnvelope{
		UCRealPropertySearch: record,
	}, nil
}

func extractObject(payload []byte, path string) (map[string]interface{}, error) {
	result := gjson.GetBytes(payload, path)
	if !result.Exists() || result.Type == gjson.Null {
		return nil, nil
	}
	if !result.IsObject() {
		return nil, fmt.Errorf("expected object at %s", path)
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(result.Raw), &out); err != nil {
		return nil, fmt.Errorf("decode object at %s: %w", path, err)
	}
	return out, nil
}

func extractArrayObjects(payload []byte, path string) ([]map[string]interface{}, error) {
	result := gjson.GetBytes(payload, path)
	if !result.Exists() || result.Type == gjson.Null {
		return nil, nil
	}
	if !result.IsArray() {
		return nil, fmt.Errorf("expected array at %s", path)
	}

	var out []map[string]interface{}
	if err := json.Unmarshal([]byte(result.Raw), &out); err != nil {
		return nil, fmt.Errorf("decode array at %s: %w", path, err)
	}
	return out, nil
}

func extractMergedArray(payload []byte, path string) ([]map[string]interface{}, error) {
	result := gjson.GetBytes(payload, path)
	if !result.Exists() || result.Type == gjson.Null {
		return nil, nil
	}
	if !result.IsArray() {
		return nil, fmt.Errorf("expected array at %s", path)
	}

	var wrappers []rowSubdataWrapper
	if err := json.Unmarshal([]byte(result.Raw), &wrappers); err != nil {
		return nil, fmt.Errorf("decode row/subdata array at %s: %w", path, err)
	}

	merged := make([]map[string]interface{}, 0, len(wrappers))
	for i, item := range wrappers {
		entry := make(map[string]interface{})
		for key, value := range item.Row {
			entry[key] = value
		}
		for key, value := range item.Subdata {
			if _, exists := entry[key]; exists {
				return nil, fmt.Errorf("duplicate key %q at %s[%d]", key, path, i)
			}
			entry[key] = value
		}
		merged = append(merged, entry)
	}

	return merged, nil
}

func parseValuation(input map[string]interface{}) SpatialistValuation {
	valuation := SpatialistValuation{}
	valuation.FMVLand, valuation.OGFMVLand = parseMoneyField(input, "FMV_LAND")
	valuation.FMVImprv, valuation.OGFMVImprv = parseMoneyField(input, "FMV_IMPRV")
	valuation.FMVTotal, valuation.OGFMVTotal = parseMoneyField(input, "FMV_TOTAL")
	valuation.ASDLand, valuation.OGASDLand = parseMoneyField(input, "ASD_Land")
	valuation.ASDBLDG, valuation.OGASDBLDG = parseMoneyField(input, "ASD_BLDG")
	valuation.ASDTotal, valuation.OGASDTotal = parseMoneyField(input, "ASD_TOTAL")
	return valuation
}

func parseSales(items []map[string]interface{}) ([]SpatialistSale, error) {
	if items == nil {
		return nil, nil
	}

	out := make([]SpatialistSale, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		sale := SpatialistSale{
			DocumentNumber: getInt64(item, "document_number"),
			Book:           getString(item, "book"),
			Page:           getString(item, "page"),
			SaleType:       getString(item, "sale_type"),
			DateOfSale:     getString(item, "date_of_sale"),
			Order0:         getString(item, "order_0"),
			Grantor:        getString(item, "grantor"),
			Grantee:        getString(item, "grantee"),
		}
		sale.GrossSellingPrice, sale.OGGrossSellingPrice = parseMoneyField(item, "gross_selling_price")
		out = append(out, sale)
	}
	return out, nil
}

func parseAssessments(items []map[string]interface{}) ([]SpatialistAssessment, error) {
	if items == nil {
		return nil, nil
	}

	out := make([]SpatialistAssessment, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		assessment := SpatialistAssessment{
			AssessDate:   getString(item, "assess_date"),
			Order0:       getString(item, "order_0"),
			ParcelYear:   getInt64(item, "parcel_year"),
			Order1:       getInt64(item, "order_1"),
			ChangeReason: getString(item, "change_reason"),
		}
		assessment.TotalAssessed, assessment.OGTotalAssessed = parseMoneyField(item, "TotalAssessed")
		out = append(out, assessment)
	}
	return out, nil
}

func parsePermits(items []map[string]interface{}) ([]SpatialistPermit, error) {
	if items == nil {
		return nil, nil
	}

	out := make([]SpatialistPermit, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		permit := SpatialistPermit{
			PermitNumber:           getInt64(item, "PermitNumber"),
			IssuedDate:             getString(item, "IssuedDate"),
			ReportCode:             getString(item, "ReportCode"),
			PermitTemplate:         getString(item, "PermitTemplate"),
			ContractorOrganization: getString(item, "ContractorOrganization"),
			PermitDescription:      getString(item, "PermitDescription"),
		}
		out = append(out, permit)
	}
	return out, nil
}

func getString(input map[string]interface{}, key string) string {
	if input == nil {
		return ""
	}
	value, ok := input[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	default:
		return ""
	}
}

func getInt64(input map[string]interface{}, key string) int64 {
	if input == nil {
		return 0
	}
	value, ok := input[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return parsed
		}
		if parsed, err := v.Float64(); err == nil {
			return int64(parsed)
		}
	case string:
		if parsed, ok := parseNumericString(v); ok {
			switch num := parsed.(type) {
			case int64:
				return num
			case float64:
				return int64(num)
			}
		}
	}
	return 0
}

func parseMoneyField(input map[string]interface{}, key string) (int64, string) {
	if input == nil {
		return 0, ""
	}
	value, ok := input[key]
	if !ok || value == nil {
		return 0, ""
	}
	switch v := value.(type) {
	case string:
		parsed, ok := parseNumericString(v)
		if !ok {
			return 0, v
		}
		switch num := parsed.(type) {
		case int64:
			return num, v
		case float64:
			return int64(num), v
		}
		return 0, v
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return parsed, ""
		}
		if parsed, err := v.Float64(); err == nil {
			return int64(parsed), ""
		}
	case float64:
		return int64(v), ""
	case int64:
		return v, ""
	case int:
		return int64(v), ""
	}
	return 0, ""
}

var numericStringRe = regexp.MustCompile(`^\d+(\.\d+)?$`)

func parseNumericString(value string) (interface{}, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, false
	}

	negative := false
	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		negative = true
		trimmed = strings.TrimSuffix(strings.TrimPrefix(trimmed, "("), ")")
		trimmed = strings.TrimSpace(trimmed)
	}

	trimmed = strings.TrimPrefix(trimmed, "$")
	trimmed = strings.ReplaceAll(trimmed, ",", "")
	if trimmed == "" || !numericStringRe.MatchString(trimmed) {
		return nil, false
	}

	if strings.Contains(trimmed, ".") {
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil, false
		}
		if negative {
			parsed = -parsed
		}
		return parsed, true
	}

	parsed, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return nil, false
	}
	if negative {
		parsed = -parsed
	}
	return parsed, true
}

package parceljob

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"github.com/tidwall/gjson"
)

// verifyParcelCapture checks receipt order and complete page membership before
// any source feature is normalized or a current row is considered for update.
func verifyParcelCapture(snapshot *parcelevidence.Snapshot) error {
	expected := 0
	offset := 0
	covered := 0
	countReceipts := 0
	for i, receipt := range snapshot.Manifest.Receipts {
		u, err := url.Parse(receipt.Request)
		if err != nil {
			return err
		}
		q := u.Query()
		body, err := snapshot.Read(i)
		if err != nil {
			return err
		}
		if q.Get("returnCountOnly") == "true" {
			if snapshot.Manifest.Kind == "full_scan" && i != 0 || snapshot.Manifest.Kind == "reprocess" && (i == 0 || !strings.HasPrefix(q.Get("where"), "PID = '") && !strings.HasPrefix(q.Get("where"), "(")) {
				return fmt.Errorf("out of order parcel count receipt")
			}
			value := gjson.GetBytes(body, "count")
			if value.Type != gjson.Number || value.Int() < 0 || strconv.FormatInt(value.Int(), 10) != value.Raw {
				return fmt.Errorf("invalid parcel count")
			}
			if countReceipts > 0 && offset != expected {
				return fmt.Errorf("incomplete PID pages")
			}
			countReceipts++
			expected = int(value.Int())
			offset = 0
			continue
		}
		if receipt.DeclaredCRS == 0 {
			return fmt.Errorf("missing page CRS")
		}
		crs, err := union.ParcelResponseCRS(body)
		if err != nil {
			return err
		}
		if crs != receipt.DeclaredCRS {
			return fmt.Errorf("receipt CRS mismatch")
		}
		if strings.HasPrefix(q.Get("where"), "OBJECTID = ") && snapshot.Manifest.Kind == "reprocess" {
			if i == 0 || countReceipts == 0 {
				continue
			}
			if offset != expected {
				return fmt.Errorf("incomplete selected PID")
			}
			continue
		}
		if countReceipts == 0 {
			return fmt.Errorf("page before count")
		}
		page := gjson.GetBytes(body, "features").Array()
		current, _ := strconv.Atoi(q.Get("resultOffset"))
		if current != offset {
			return fmt.Errorf("missing or duplicated page offset")
		}
		size, err := strconv.Atoi(q.Get("resultRecordCount"))
		if err != nil || size <= 0 {
			return fmt.Errorf("invalid page limit")
		}
		if len(page) != min(size, expected-offset) {
			return fmt.Errorf("incomplete parcel page")
		}
		offset += len(page)
		covered += len(page)
	}
	if countReceipts == 0 || offset != expected || covered != snapshot.Manifest.Count {
		return fmt.Errorf("incomplete parcel snapshot")
	}
	return nil
}

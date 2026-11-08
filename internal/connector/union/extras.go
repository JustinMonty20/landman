package union

import (
	"encoding/json"
	"fmt"
	"time"
)

// UnionParcelExtras contains retained Union County GIS fields that are not part
// of the shared normalized parcel concepts.
type UnionParcelExtras struct {
	OBJECTID     *int64     `json:"OBJECTID,omitempty"`
	OBJECTID_1   *int64     `json:"OBJECTID_1,omitempty"`
	SUBCODE      *string    `json:"SUBCODE,omitempty"`
	DIST_TWN     *string    `json:"DIST_TWN,omitempty"`
	DIST_CODE    *string    `json:"DIST_CODE,omitempty"`
	TAX_DISTRI   *string    `json:"TAX_DISTRI,omitempty"`
	TWSHP_CD     *string    `json:"TWSHP_CD,omitempty"`
	TWSHP_DESC   *string    `json:"TWSHP_DESC,omitempty"`
	DESC1_DESC   *string    `json:"DESC1_DESC,omitempty"`
	DESC1_CODE   *string    `json:"DESC1_CODE,omitempty"`
	LUV_YES_NO   *string    `json:"LUV_YES_NO,omitempty"`
	MULSTR       *string    `json:"MULSTR,omitempty"`
	MULT_LANDSEQ *string    `json:"MULT_LANDSEQ,omitempty"`
	NBHDNAME     *string    `json:"NBHDNAME,omitempty"`
	NBHDNUM      *string    `json:"NBHDNUM,omitempty"`
	NBHNEW       *string    `json:"NBHNEW,omitempty"`
	IDCOL        *int64     `json:"idcol,omitempty"`
	DATE_CRT     *time.Time `json:"DATE_CRT,omitempty"`
	OG_DATE_CRT  *int64     `json:"og_DATE_CRT,omitempty"`
}

func marshalSourceExtras(extras UnionParcelExtras) (json.RawMessage, error) {
	payload, err := json.Marshal(struct {
		Union UnionParcelExtras `json:"union"`
	}{Union: extras})
	if err != nil {
		return nil, fmt.Errorf("marshal union source extras: %w", err)
	}
	return payload, nil
}

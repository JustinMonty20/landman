package parcelstore

import (
	"encoding/json"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

func historyNumber(exact string, fallback *float64) any {
	if exact != "" {
		return json.Number(exact)
	}
	return fallback
}

// numericValue binds exact scaled source values without a binary float round trip.
func numericValue(exact string, fallback *float64) any {
	if exact == "" {
		return fallback
	}
	point := strings.IndexByte(exact, '.')
	fraction := 0
	if point >= 0 {
		fraction = len(exact) - point - 1
		exact = exact[:point] + exact[point+1:]
	}
	integer, ok := new(big.Int).SetString(exact, 10)
	if !ok {
		return fallback
	}
	return pgtype.Numeric{Int: integer, Exp: -int32(fraction), Valid: true}
}

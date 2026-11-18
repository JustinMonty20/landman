// Package parcelevidence defines the parcel source evidence and fingerprint contracts.
package parcelevidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

const SourceHashVersion = "json-rational-directed-rings-v2"
const NormalizerVersion = "union-parcel-v1"

// SourceHash hashes decoded JSON values, not locally observed retrieval details.
// Numbers are reduced as exact decimal rationals before hashing.
func SourceHash(feature []byte) (string, error) {
	d := json.NewDecoder(bytes.NewReader(feature))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return "", err
	}
	var b strings.Builder
	if err := canonical(&b, value, ""); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(SourceHashVersion + "\n" + b.String()))
	return hex.EncodeToString(sum[:]), nil
}

func canonical(b *strings.Builder, value any, key string) error {
	switch v := value.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if v {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		encoded, _ := json.Marshal(v)
		b.Write(encoded)
	case json.Number:
		r, ok := new(big.Rat).SetString(string(v))
		if !ok {
			return fmt.Errorf("invalid JSON number %q", v)
		}
		b.WriteString(r.RatString())
	case []any:
		if key == "rings" {
			return canonicalRings(b, v)
		}
		b.WriteByte('[')
		for i, item := range v {
			if i != 0 {
				b.WriteByte(',')
			}
			if err := canonical(b, item, ""); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i != 0 {
				b.WriteByte(',')
			}
			encoded, _ := json.Marshal(k)
			b.Write(encoded)
			b.WriteByte(':')
			if err := canonical(b, v[k], k); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("unsupported canonical JSON type %T", v)
	}
	return nil
}

// canonicalRings changes only ring start and component order. ArcGIS ring
// direction identifies shells and holes, so reversing it changes source meaning.
// It never rounds coordinates or removes vertices.
func canonicalRings(b *strings.Builder, rings []any) error {
	encoded := make([]string, 0, len(rings))
	for _, raw := range rings {
		points, ok := raw.([]any)
		if !ok || len(points) < 4 {
			return fmt.Errorf("invalid polygon ring")
		}
		vertex := make([]string, len(points))
		for i, point := range points {
			var tmp strings.Builder
			if err := canonical(&tmp, point, ""); err != nil {
				return err
			}
			vertex[i] = tmp.String()
		}
		if vertex[0] != vertex[len(vertex)-1] {
			return fmt.Errorf("unclosed polygon ring")
		}
		vertex = vertex[:len(vertex)-1]
		forward := minimalRotation(vertex)
		encoded = append(encoded, "["+strings.Join(append(forward, forward[0]), ",")+"]")
	}
	sort.Strings(encoded)
	b.WriteByte('[')
	b.WriteString(strings.Join(encoded, ","))
	b.WriteByte(']')
	return nil
}

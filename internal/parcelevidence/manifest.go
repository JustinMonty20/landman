package parcelevidence

import "time"

// Receipt records a source response bound by the shared capture registry.
type Receipt struct {
	Path        string    `json:"path"`
	Request     string    `json:"request"`
	RetrievedAt time.Time `json:"retrieved_at"`
	Bytes       int       `json:"bytes"`
	SHA256      string    `json:"sha256"`
	DeclaredCRS int       `json:"declared_crs,omitempty"`
	Validated   bool      `json:"validated"`
}

// Manifest retains parcel-specific observation evidence alongside the shared capture.
type Manifest struct {
	ArtifactHash        string    `json:"artifact_hash"`
	ArtifactHashVersion string    `json:"artifact_hash_version"`
	Version             int       `json:"version"`
	Source              string    `json:"source"`
	Kind                string    `json:"kind"`
	ID                  string    `json:"id"`
	CreatedAt           time.Time `json:"created_at"`
	Count               int       `json:"count"`
	Receipts            []Receipt `json:"receipts"`
}

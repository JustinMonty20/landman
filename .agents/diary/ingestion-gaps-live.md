# Live parcel capture

- **Scope tangent worth pursuing later:** The approved capture replay stores 28 geometries for which `ST_IsValid` is false. Both imports succeed without quarantine and preserve identical EWKB and topology results. Keep geometry repair and broader topology rejection outside this verification, as required by `plans/parcel-geometry-topology.md`.

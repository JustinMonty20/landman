# Compact parcel detail preferences

Account preferences are one ordered allowlisted `text[]` column on `app_users`. Its default is `acreage,soil`; an empty array is intentional. GET `/api/me/parcel-detail-groups` returns `{ "groups": [...] }`. PUT to the same endpoint accepts only `{ "groups": [...] }` with distinct known IDs, at most 512 body bytes, JSON content type and exact same-origin header. Both operations use the authenticated session's current user ID, never a supplied account ID. PUT canonicalizes to acreage, ownership, tax, vacancy, soil, zoning, roads, flood order. Reads from `/api/v1/parcels/*` remain unchanged.

The detail panel always fetches parcel identity, displays number and address, and fetches only selected separable tax, vacancy, soil, zoning, roads and flood endpoints. Acreage and ownership reuse the identity response. Detail visibility does not change map-layer switches. The map's FEMA release comparison remains when flood detail is shown. Failed saves retain the editor and announce failure; default display on failed preference reads is explicitly unsaved.

Checks selected: focused auth PostGIS tests for isolation, validation, default and empty; browser desktop/tablet for persistence, request counts and panel close/list; UI check/build and focused Go tests. No live database or publication. Receipts outside Git.

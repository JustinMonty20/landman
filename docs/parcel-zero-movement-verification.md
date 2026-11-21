# Verify zero-movement parcel repairs

The verification build tests a proposed repair without changing normal imports. A feature qualifies only when both PostGIS repair methods return the same valid polygonal geometry and retain every directed boundary segment. The parcel writer validates the complete parcel after the union. A refusal or database failure rolls back the replay transaction. The verification build does not publish rejection state for readers.

Use only a fresh disposable PostGIS database named `landman_parcel_repair_test` on a loopback port. Apply the repository migrations. Do not load `.env`. Copy only the required capture registrations and their originating attempt records. Use a private copy of the artifact files. Build the verification executable:

```sh
go build -tags parcelrepairverify -o "$VERIFY_BIN" ./cmd/landman
```

Run `replay union.gis.parcels --snapshot UUID` with the disposable `DATABASE_URL` and the private `PARCEL_EVIDENCE_DIR`. The tagged executable refuses all other modes. Do not deploy this executable. The normal build does not call the repair rule.

Use the normal executable to rebuild a baseline in the disposable database. Replay the same capture with the verification executable. Compare geometry validity, the exact geometry bytes of the unaffected parcels, parcel and sale IDs, and tax links. Verify that the source observation does not change. Repeat the correction and verify that it does not add more history or derived work. Keep capture IDs and per-parcel logs private.

The follow-up change will define production rejection state, update spatial consumers, and remove the verification-only restriction. Do not enable normal imports with this rule before those changes pass verification.

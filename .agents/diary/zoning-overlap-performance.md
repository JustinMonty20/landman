[Scope tangent, resolved] Step 10 reduced polygon precision for each parcel match. A sample of 100 matches took about 2.4 seconds with repeated preparation and 0.02 seconds with one preparation. Step 10 now prepares each jurisdiction polygon once.

[Scope tangent] Overlap tables still store a full polygon for each match. Measure temporary table size and step times before replacing these copies with polygon IDs.

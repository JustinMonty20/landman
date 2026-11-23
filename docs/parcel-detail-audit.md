# Parcel detail audit

## Scope

Audit the 22 fields in the parcel detail picker and the parcel number and site address. This audit does not propose removing fields from storage or from API responses. Keep individual field selection. The recommendations below concern what the panel shows and where it shows it.

## Zoning sources

- **Weddington district and use rules.** The town publishes its [Unified Development Ordinance](https://www.townofweddington.com/unified-development-ordinance). The PDF linked from that page contains [Article 7, Table 1, Permitted Uses](https://www.townofweddington.com/sites/default/files/uploads/documents/amended_udo_2026-10-12_1.pdf#page=43), on PDF pages 43 and 44. Table 2 contains lot and building standards. Table 1 lists single-family dwellings as by right in both R-40 and R-CD. This is a district rule, not approval for this parcel or proof that it can support a dwelling.
- **Mapped boundaries.** The [town planning documents page](https://www.townofweddington.com/planning-documents-and-maps) links to its zoning map. The linked map file is dated 2022. Do not use it alone to confirm a current parcel assignment. The ordinance answers what each district allows. The map and any parcel-specific case or ordinance answer which rules apply at the site.
- **Panel recommendation.** Show the source jurisdiction, district name and code, mapped acres, and any verified case reference for each overlap. Link the district to the official ordinance and label the use table. Show overlapping assignments and any uncovered area separately. Verify the source meaning of `CITY` before labeling it as a district. Do not translate a district code without checking the jurisdiction. If an exact ordinance link is not verified, link the official planning page and say what is unknown.

## Union County zoning inventory

A read-only check of the local published zoning set found 55,081 stored features and 164 distinct layer, jurisdiction, and code combinations. These counts describe stored data, not verified current countywide coverage. No feature in this set has a stored official link.

| Source layer | Jurisdiction | Features | Codes | Features with a description |
| --- | --- | ---: | ---: | ---: |
| 173 | Fairview | 14 | 3 | 0 |
| 174 | Indian Trail | 18,251 | 26 | 0 |
| 175 | Lake Park | 151 | 13 | 0 |
| 176 | Marshville | 1,911 | 13 | 1,807 |
| 177 | Marvin | 263 | 11 | 0 |
| 180 | Monroe | 18,551 | 21 | 0 |
| 181 | Stallings | 7,159 | 14 | 0 |
| 190 | Union County | 721 | 27 | 0 |
| 183 | Waxhaw | 7,971 | 8 | 7,299 |
| 184 | Weddington | 22 | 12 | 0 |
| 186 | Wingate | 67 | 16 | 0 |

The stored boundary layer also names Hemby Bridge, Mineral Springs, Mint Hill, Unionville, and Wesley Chapel. There is no separate zoning layer for these names in the published set. This does not prove those areas have no zoning or that county rules apply. Verify the planning authority and map source before assigning a district. The county layer contains 167 `CITY` features, with no description or official link. Keep `CITY` as an unverified source value, not a district with permitted uses. Direct requests to the county planning site and a candidate ordinance publisher returned HTTP 403 during this audit. This does not establish that the county has no public ordinance or prohibit a buyer from opening it normally. Do not bypass the refusal or automate ingestion from those paths.

## Link first, then add verified descriptions

**Union County link for the first release.** Use the [county Unified Development Ordinance page](https://www.unioncountync.gov/government/departments-f-p/planning-development/unified-development-ordinance) for county zoning. The operator opened the 295-page document. This environment received HTTP 403, so it did not verify the document date or page references. Use the [county zoning map](https://ucnc-gis.maps.arcgis.com/apps/webappviewer/index.html?id=a1362cc333e04c79a176b1eb6feeddf0) as a separate link. Use town sources where town rules apply. Do not fetch the ordinance when the buyer opens parcel details.

**Deferred document import.** Start with a PDF that the operator downloads. Consider an S3 upload location for a later delivery. Keep the original file and its official source link. Record the document version or date when verified. Extract district descriptions, use tables, and development standards with section and page references. Require review before the application shows any extracted description as a zoning rule. No S3 setup or parser is part of the link delivery.

1. For each jurisdiction, verify the official planning page, current ordinance, official map, and the use table if one exists. Retain the official link, document date, and section or page. Show a verified planning page when no district link is known. Weddington has verified links; the other source links still need verification.
2. Join each stored zoning feature to a verified district by planning authority and district code. Keep the original source code. Confirm aliases against the ordinance. For example, Weddington GIS uses `R-40D`, while the ordinance uses `R-40(D)`. Do not map an alias from spelling alone.
3. Start with short, reviewed descriptions for verified districts. Link every description to its official source. Keep conditional districts and site-specific case documents separate. Show all mapped overlaps and distinguish split zoning from conflicting assignments. Do not infer allowed uses where coverage, authority, or the district match is uncertain.
4. Consider automated document import only after a source check confirms a repeatable access path, version and date checks, and permission to retain or redistribute the material. A buyer link does not need document import. Do not make PDF extraction a condition for the first links.

## Display decisions

| Fields | Buyer value | Recommendation |
| --- | --- | --- |
| Parcel number and site address | Identify the parcel and find the county record. | Always show. Distinguish the site address from the mailing address. |
| Mapped acreage | Compare parcel size with the buyer's target. | Keep. Round to two decimals. Identify the source as a map, not a survey. |
| Owner name; entity indicator | Identify the name on the county record. A name suffix can suggest an entity. | Keep the name. Put the indicator in owner details. It does not verify registration or title. |
| Stored sale count | Count alone does not explain the sale price or date. | Replace the count row with collapsible sales history. Show the latest dated stored sale. List stored dates, prices, and deed references when expanded. Do not imply complete history or market value. |
| Reported delinquent total; stored tax bill count; foreclosure case count | Reported debt or a foreclosure record can justify a county records check. The bill count alone does not help that decision. | Keep the reported balance with its date. Move bills to supporting records. Replace the foreclosure count with a dated record summary when supported. Missing records do not mean zero debt or no open case. |
| Vacancy tier; vacancy reason | Help a buyer decide whether to inspect current use. | Show the tier with its reason when selected. Label it provisional. Do not imply that the site is unoccupied. |
| Assessed soil acreage | Helps check how much of the parcel the analysis represents. Total parcel area alone does not show mapped coverage. | Move it to soil details and label the measured area correctly. Show uncovered or ambiguous area when present. |
| Septic field rating; dwelling without basement rating; dwelling with basement rating | Flag constraints for three different uses. | Keep each selectable. Show named limits, a readable area, and the next site check. Show stale status once in the soil section. This is mapped screening, not site approval. |
| Zoning district; jurisdiction; unmapped acreage | Identify the authority, rules, and limits of the map. | Keep district and jurisdiction. Put unmapped area in coverage details unless it is positive or unknown. Show the official links above. |
| Mapped road distance; nearest road name; road status | Identify the nearest mapped road and guide an access check. | Keep name and distance. Put source status in details. Neither distance nor active status proves frontage, a driveway, or legal access. |
| Unknown flood coverage acreage; mapped floodway acreage; mapped flood zones | Flag mapped hazard and gaps. | Keep the zone and subtype. Show floodway and coverage gaps first when present. Put zero-area checks and source dates in details. Zone X alone does not distinguish minimal hazard from the 0.2% annual-chance area. |

## Proposed additional field

**Owner mailing address.** Return the stored county mailing address as an independent field in the signed-in parcel panel. Use it to identify the address on the county record. It does not identify the owner's residence or verify that mail will reach the owner. Do not add exports or owner contact actions.

## External parcel links

- **Open Union County property record.** Use `https://property.spatialest.com/nc/union#/property/{parcel_id}` as a browser link only. Verify that a real parcel opens with the county parcel number, not the Landman UUID, before adding the link to the panel. Preserve leading zeros. The site home page responded, but this audit did not verify a parcel deep link. Do not fetch or scrape records through this feature.
- **Open satellite view.** Keep this separate from the property record. The existing browser plan specifies Google Maps in satellite mode, centered on a point inside the parcel. Do not send owner data to either site.

## Rules for the panel

Show each source date and limit once per section. Use calendar dates. Show a positive area below 0.01 acre as `<0.01 acres`. Keep exact zero, unknown coverage, missing evidence, and stale evidence distinct. Keep all fields selected by default, but do not give supporting details the same visual weight as a result. These are audit recommendations. No UI or API change is part of this audit.

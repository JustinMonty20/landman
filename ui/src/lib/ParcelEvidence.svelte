<script lang="ts">
import { readMapData, type FloodDetail } from './flood';
import type { DetailField } from './detailGroups';
let { id, fields, releaseId, onlocation, onresearch }: { id: string; fields: DetailField[]; releaseId?: string; onlocation: (id: string, point: [number, number] | null) => void; onresearch: (id: string, parcelNumber: string | null, coordinates: [number, number] | null) => void } = $props();
type Section<T> = { status: 'loading' } | { status: 'unavailable' } | { status: 'ready'; data: T };
type Facts = { location?: { coordinates: [number, number] } | null; parcel: { parcel_number: string | null; address: string | null; mapped_acres: string | null; updated_at: string }; source: string; owner_labels: null | { entity_name_indicator: string | null }; valuation?: { land: string | null; improvement: string | null; total: string | null; assessed: string | null }; sales: { date: string | null; amount: string | null; deed_book: string | null; deed_page: string | null }[] };
type Tax = { bills: { year: number; total_taxes: string; fetched_at: string }[]; foreclosure_cases: { case_number: string | null; status: string | null; sale_on: string | null; fetched_at: string | null }[]; pipeline?: { case_number: string | null; status: string | null; sale_on: string | null; fetched_at: string | null }[]; delinquent_balances?: { year: number; balance_due: string; fetched_at: string }[]; reported_delinquent_total: string | null; note: string };
type Vacancy = { available: boolean; tier: string; reason: string; facts?: Record<string, unknown>; structure_source_as_of?: string; assessment_source_as_of?: string; structures_retrieved_at?: string; assessment_retrieved_at?: string };
type Soil = { calculated_at?: string; available: boolean; reason?: string; total_acres: number | null; uses: { use: string; rated: { rating: string; area: { acres: number } }[]; unrated: { acres: number }; uncovered: { acres: number }; ambiguous: { acres: number }; next_step: string; map_units?: { map_unit_key: string; interpretation: { fetched_at?: string; revision?: { survey_saved_on?: string | null }; components?: { name: string; reason_name?: string; reason?: string }[] } }[] }[]; freshness: { stale: boolean; reasons: string[]; refresh_failed_at: string | null } };
type Freshness = { stale: boolean; reasons: string[]; source_fetched_at: string | null; refresh_failed_at: string | null };
type Zoning = { available: boolean; unmapped_acres: number; zones: { jurisdiction: string; district_code: string | null; description?: string | null; case_reference?: string | null; area: { acres: number } }[]; jurisdictions: { name: string | null }[]; freshness: Freshness };
type Roads = { available: boolean; state: string; distance_feet: number | null; ties: { name?: string | null; Name?: string | null; status?: string | null; Status?: string | null }[]; freshness: Freshness };
type Owner = { current_name_1: string | null; current_name_2: string | null; mailing_line_1?: string | null; mailing_line_2?: string | null; mailing_city?: string | null; mailing_state?: string | null; mailing_postal_code?: string | null };
let facts = $state<Section<Facts>>({ status: 'loading' });
let owner = $state<Section<Owner>>({ status: 'loading' });
let tax = $state<Section<Tax>>({ status: 'loading' });
let vacancy = $state<Section<Vacancy>>({ status: 'loading' });
let soil = $state<Section<Soil>>({ status: 'loading' });
let flood = $state<Section<FloodDetail>>({ status: 'loading' });
let zoning = $state<Section<Zoning>>({ status: 'loading' });
let roads = $state<Section<Roads>>({ status: 'loading' });
const has = (field: DetailField) => fields.includes(field);
function load<T>(id: string, path: string, assign: (state: Section<T>) => void) {
 const task = new AbortController();
 assign({ status: 'loading' });
 readMapData<T>(path, task.signal).then(data => { if (!task.signal.aborted) assign({ status: 'ready', data }); })
 .catch(() => { if (!task.signal.aborted) assign({ status: 'unavailable' }); });
 return () => task.abort();
}
$effect(() => { if (!id) return; const selected = id; return load<Facts>(id, `/api/v1/parcels/${encodeURIComponent(id)}`, state => {
 facts = state;
 if (state.status === 'ready') { onlocation(selected, state.data.location?.coordinates ?? null); onresearch(selected, state.data.parcel.parcel_number, state.data.location?.coordinates ?? null); }
 if (state.status === 'unavailable') { onlocation(selected, null); onresearch(selected, null, null); }
}); });
$effect(() => { if (id && (has('owner_name') || has('owner_mailing'))) return load<Owner>(id, `/api/me/parcel-owner/${encodeURIComponent(id)}`, state => owner = state); });
$effect(() => { if (id && (has('delinquent_total') || has('tax_bill_count') || has('foreclosure_count'))) return load<Tax>(id, `/api/v1/parcels/${encodeURIComponent(id)}/tax`, state => tax = state); });
$effect(() => { if (id && (has('vacancy_tier') || has('vacancy_reason'))) return load<Vacancy>(id, `/api/v1/parcels/${encodeURIComponent(id)}/vacancy`, state => vacancy = state); });
$effect(() => { if (id && (has('soil_acres') || has('soil_septic') || has('soil_dwelling_without') || has('soil_dwelling_with'))) return load<Soil>(id, `/api/v1/parcels/${encodeURIComponent(id)}/soil`, state => soil = state); });
$effect(() => { if (id && (has('flood_unknown') || has('floodway_acres') || has('flood_zones'))) return load<FloodDetail>(id, `/api/v1/parcels/${encodeURIComponent(id)}/flood`, state => flood = state); });
$effect(() => { if (id && (has('zoning_district') || has('zoning_jurisdiction') || has('zoning_unmapped'))) return load<Zoning>(id, `/api/v1/parcels/${encodeURIComponent(id)}/zoning`, state => zoning = state); });
$effect(() => { if (id && (has('road_distance') || has('road_name') || has('road_status'))) return load<Roads>(id, `/api/v1/parcels/${encodeURIComponent(id)}/roads`, state => roads = state); });
const text = (value: unknown) => value == null || value === '' ? 'Not recorded' : String(value);
const areaText = (value: string | null) => value == null ? 'Not recorded' : acres(Number(value));
const acres = (value: number | null | undefined) => value == null ? 'Not recorded' : value > 0 && value < 0.01 ? '<0.01' : value.toFixed(2);
const date = (value: string | null | undefined) => value ? /^\d{4}-\d{2}-\d{2}/.test(value) ? value.slice(0,10) : value : 'Date not recorded';
const money = (value: string | null | undefined) => value == null ? 'Not recorded' : '$' + Number(value).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
const freshness = (value: Freshness) => `${value.stale ? 'Stale' : 'Stored source'} · Fetched ${date(value.source_fetched_at)}${value.refresh_failed_at ? ' · Refresh failed ' + date(value.refresh_failed_at) : ''}${value.reasons.length ? ' · ' + value.reasons.join(', ') : ''}`;
const soilUse = (field: DetailField) => ({ soil_septic: 'conventional-septic', soil_dwelling_without: 'dwelling-without-basement', soil_dwelling_with: 'dwelling-with-basement' } as Partial<Record<DetailField, string>>)[field];
</script>
{#snippet status<T>(section: Section<T>)}
  {#if section.status === 'loading'}<p>Loading evidence…</p>
  {:else if section.status === 'unavailable'}<p>Evidence temporarily unavailable.</p>{/if}
{/snippet}
<section aria-label="Parcel identity">
  <h2>Parcel</h2>
  {@render status(facts)}
  {#if facts.status === 'ready'}
    <p>{text(facts.data.parcel.parcel_number)} · Site address: {text(facts.data.parcel.address)}</p>

  {/if}
</section>
{#if has('mapped_acres') || has('valuation_land') || has('valuation_improvement') || has('valuation_total') || has('valuation_assessed')}
<section class="evidence-group" aria-label="Land and value">
  <h2>Land and value</h2>
  {@render status(facts)}
  {#if facts.status === 'ready'}
    <dl>
      {#if has('mapped_acres')}<dt>Mapped acreage</dt><dd>{areaText(facts.data.parcel.mapped_acres)} acres</dd>{/if}
      {#each [{ id: 'valuation_land', title: 'Parcel land value', key: 'land' }, { id: 'valuation_improvement', title: 'Parcel improvement value', key: 'improvement' }, { id: 'valuation_total', title: 'Parcel total value', key: 'total' }, { id: 'valuation_assessed', title: 'Parcel assessed value', key: 'assessed' }] as value}
        {#if has(value.id as DetailField)}<dt>{value.title}</dt><dd>{money(facts.data.valuation?.[value.key as keyof NonNullable<Facts['valuation']>])}</dd>{/if}
      {/each}
    </dl>
    <details><summary>Source and limits</summary><p>{facts.data.source} · Updated {date(facts.data.parcel.updated_at)}. {has('mapped_acres') ? 'Mapped area is not a survey. ' : ''}{has('valuation_land') || has('valuation_improvement') || has('valuation_total') || has('valuation_assessed') ? 'Stored county values are not a current appraisal or separate assessed-building layer.' : ''}</p></details>
  {/if}
</section>
{/if}
{#if has('owner_name') || has('owner_mailing') || has('entity_indicator') || has('sale_count')}
<section class="evidence-group" aria-label="Ownership and sales">
  <h2>Ownership and sales</h2>
  {#if has('owner_name') || has('owner_mailing')}{@render status(owner)}{/if}
  {#if has('entity_indicator') || has('sale_count')}{@render status(facts)}{/if}
  <dl>
    {#if has('owner_name') && owner.status === 'ready'}<dt>Owner name</dt><dd>{[owner.data.current_name_1, owner.data.current_name_2].filter(Boolean).join(' · ') || 'Not recorded'}</dd>{/if}
    {#if has('owner_mailing') && owner.status === 'ready'}<dt>Owner mailing address</dt><dd>{[owner.data.mailing_line_1, owner.data.mailing_line_2, owner.data.mailing_city, owner.data.mailing_state, owner.data.mailing_postal_code].filter(Boolean).join(', ') || 'Not recorded'}</dd>{/if}
    {#if has('sale_count') && facts.status === 'ready'}
      {@const latest = [...facts.data.sales].filter(item => item.date).sort((a,b) => (b.date ?? '').localeCompare(a.date ?? ''))[0]}
      <dt>Sales history</dt><dd>{latest ? `Latest dated sale ${date(latest.date)} · ${money(latest.amount)}` : 'No dated sale recorded'}</dd>
    {/if}
  </dl>
  {#if has('entity_indicator') && facts.status === 'ready'}<details><summary>Entity indicator</summary><p>{text(facts.data.owner_labels?.entity_name_indicator)} · Inferred from county name labels; not verified entity ownership.</p></details>{/if}
  {#if has('sale_count') && facts.status === 'ready'}<details><summary>Stored sale records ({facts.data.sales.length})</summary>{#each facts.data.sales as sale}<p>{date(sale.date)} · {money(sale.amount)} · Deed {text(sale.deed_book)}/{text(sale.deed_page)}</p>{/each}<small>Stored sales may not be complete or reflect market value.</small></details>{/if}
  {#if has('owner_name') || has('owner_mailing')}<small>County names and mailing address are not verified contacts or residence.</small>{/if}
</section>
{/if}
{#if has('delinquent_total') || has('tax_bill_count') || has('foreclosure_count')}
<section class="evidence-group" aria-label="Tax and foreclosure">
  <h2>Tax and foreclosure</h2>{@render status(tax)}
  {#if tax.status === 'ready'}
    <dl>
      {#if has('delinquent_total')}<dt>Reported delinquent total</dt><dd>{money(tax.data.reported_delinquent_total)} · Fetched {date((tax.data.delinquent_balances ?? []).map(b => b.fetched_at).sort().at(-1))}. Feed records only; absent years do not prove payment. Not a current payoff quote.</dd>{/if}
      {#if has('foreclosure_count')}<dt>Foreclosure records</dt><dd>{tax.data.foreclosure_cases.length + (tax.data.pipeline?.length ?? 0)} stored feed records. Confirm status at county; missing records do not mean no open case.</dd>{/if}
    </dl>
    {#if has('delinquent_total')}<details><summary>Reported delinquent years</summary>{#each tax.data.delinquent_balances ?? [] as balance}<p>{balance.year} · {money(balance.balance_due)} · Fetched {date(balance.fetched_at)}</p>{/each}</details>{/if}
    {#if has('tax_bill_count')}<details><summary>Tax bills ({tax.data.bills.length} stored)</summary>{#each tax.data.bills as bill}<p>{bill.year} · Billed {money(bill.total_taxes)} · Fetched {date(bill.fetched_at)}</p>{/each}<small>Billed amounts are not unpaid balances.</small></details>{/if}
    {#if has('foreclosure_count')}<details><summary>Case and pipeline details</summary>{#each [...tax.data.foreclosure_cases, ...(tax.data.pipeline ?? [])] as item}<p>Case {text(item.case_number)} · {text(item.status)} · Sale {date(item.sale_on)} · Fetched {date(item.fetched_at)}</p>{/each}</details>{/if}
  {/if}
</section>
{/if}
{#if has('vacancy_tier') || has('vacancy_reason')}
<section class="evidence-group" aria-label="Vacancy screening">
  <h2>Vacancy screening</h2>{@render status(vacancy)}
  {#if vacancy.status === 'ready'}
    <dl>
      {#if has('vacancy_tier')}<dt>Vacancy tier</dt><dd>{vacancy.data.available ? text(vacancy.data.tier) : 'Unknown · no current validated assessment'}</dd>{/if}
      {#if has('vacancy_reason')}<dt>Vacancy reason</dt><dd>{vacancy.data.available ? text(vacancy.data.reason).replaceAll('_', ' ') : 'No published vacancy evidence'}</dd>{/if}
    </dl>
    {#if vacancy.data.available && vacancy.data.facts}<details><summary>Contributing vacancy evidence</summary>{#each Object.entries(vacancy.data.facts) as [label, value]}<p>{label.replaceAll('_',' ')}: {value == null ? 'unknown' : String(value)}</p>{/each}<p>Structures source as of: {text(vacancy.data.structure_source_as_of)} · retrieved {text(vacancy.data.structures_retrieved_at)}</p><p>Assessment source as of: {text(vacancy.data.assessment_source_as_of)} · retrieved {text(vacancy.data.assessment_retrieved_at)}</p></details>{/if}
    <small>Provisional screening, not occupancy verification.</small>
  {/if}
</section>
{/if}
{#if has('soil_acres') || has('soil_septic') || has('soil_dwelling_without') || has('soil_dwelling_with')}
<section class="evidence-group" aria-label="Soil screening">
  <h2>Soil screening</h2>{@render status(soil)}
  {#if soil.status === 'ready'}
    {#if !soil.data.available}<p>No published soil assessment · {text(soil.data.reason)}</p>
    {:else}
      <small>{soil.data.freshness.stale ? 'Stale soil evidence' : 'Stored soil evidence'} · Calculated {date(soil.data.calculated_at)} · {soil.data.freshness.reasons.join(', ')} · Screening only; confirm site with county health department.</small>
      <dl>
        {#each ['soil_septic','soil_dwelling_without','soil_dwelling_with'] as field}
          {#if has(field as DetailField)}
            {@const use = soil.data.uses.find(item => item.use === soilUse(field as DetailField))}
            <dt>{field === 'soil_septic' ? 'Septic field rating' : field === 'soil_dwelling_without' ? 'Dwelling without basement rating' : 'Dwelling with basement rating'}</dt>
            <dd>{use ? use.rated.map(r => `${r.rating}: ${acres(r.area.acres)} acres`).join('; ') || 'No rated area' : 'No use-specific rating recorded'}
              {#if use && (use.unrated.acres > 0 || use.uncovered.acres > 0 || use.ambiguous.acres > 0)}<span class="warning"> · Coverage gap: unrated {acres(use.unrated.acres)}, uncovered {acres(use.uncovered.acres)}, ambiguous {acres(use.ambiguous.acres)} acres.</span>{/if}
            </dd>
          {/if}
        {/each}
      </dl>
      <details><summary>Measured geometry, limits and next site check</summary>
        {#if has('soil_acres')}<p>Measured parcel polygon area {acres(soil.data.total_acres)} acres. This is not rated soil coverage or county parcel acreage.</p>{/if}
        {#each ['soil_septic','soil_dwelling_without','soil_dwelling_with'] as field}
          {#if has(field as DetailField)}
            {@const use = soil.data.uses.find(item => item.use === soilUse(field as DetailField))}
            {#if use}<p>{field.replaceAll('_',' ')}: {use.next_step}</p>
              {#each use.map_units ?? [] as unit}<p>Map unit {unit.map_unit_key} · USDA fetched {date(unit.interpretation.fetched_at)} · Survey saved {date(unit.interpretation.revision?.survey_saved_on)}</p>
                {#each unit.interpretation.components ?? [] as component}{#if component.reason || component.reason_name}<p>{component.name}: {text(component.reason_name)} · {text(component.reason)} (component reason, not located parcel acreage)</p>{/if}{/each}
              {/each}
            {/if}
          {/if}
        {/each}
      </details>
    {/if}
  {/if}
</section>
{/if}
{#if has('zoning_district') || has('zoning_jurisdiction') || has('zoning_unmapped')}
<section class="evidence-group" aria-label="Zoning">
  <h2>Zoning</h2>{@render status(zoning)}
  {#if zoning.status === 'ready'}
    {#if !zoning.data.available}<p>No published zoning assessment</p>{:else}
      <dl>
        {#if has('zoning_district')}<dt>Zoning district</dt><dd>{zoning.data.zones.map(z => `${text(z.district_code)} (${acres(z.area?.acres)} acres)${z.description ? ' · ' + z.description : ''}${z.case_reference ? ' · Case ' + z.case_reference : ''}`).join('; ') || 'No district recorded'}{#if zoning.data.zones.some(z => z.district_code === 'CITY')} · CITY is an unverified source code, not a verified district.{/if}</dd>{/if}
        {#if has('zoning_jurisdiction')}<dt>Jurisdiction</dt><dd>{zoning.data.jurisdictions.map(j => text(j.name)).join(', ') || 'Not recorded'}</dd>{/if}
        {#if has('zoning_unmapped') && zoning.data.unmapped_acres > 0}<dt>Zoning coverage gap</dt><dd class="warning">{acres(zoning.data.unmapped_acres)} acres</dd>{/if}
      </dl>
      <details><summary>Coverage, authority and sources</summary>
        {#if has('zoning_unmapped')}<p>Unmapped acreage {acres(zoning.data.unmapped_acres)} acres.</p>{/if}
        {#if has('zoning_district')}{#each zoning.data.zones as zone}<p>Mapped district {text(zone.district_code)} · {zone.jurisdiction} · {acres(zone.area?.acres)} acres.</p>{/each}{/if}
        {#if has('zoning_district') || has('zoning_jurisdiction')}
          {#if zoning.data.zones.some(z => z.jurisdiction === 'Union County')}<a href="https://www.unioncountync.gov/government/departments-f-p/planning-development/unified-development-ordinance" target="_blank" rel="noopener noreferrer">Union County official ordinance page</a> · <a href="https://ucnc-gis.maps.arcgis.com/apps/webappviewer/index.html?id=a1362cc333e04c79a176b1eb6feeddf0" target="_blank" rel="noopener noreferrer">County zoning map</a>.{/if}
          {#if zoning.data.zones.some(z => z.jurisdiction === 'Weddington')}<a href="https://www.townofweddington.com/unified-development-ordinance" target="_blank" rel="noopener noreferrer">Weddington ordinance and use table</a>.{/if}
          {#if !zoning.data.zones.some(z => z.jurisdiction === 'Union County' || z.jurisdiction === 'Weddington')}Official planning link for this mapped jurisdiction is not verified.{/if}
          <p>Links do not confirm parcel assignment or allowed use.</p>
        {/if}
        <p>{freshness(zoning.data.freshness)}</p>
      </details>
    {/if}
  {/if}
</section>
{/if}
{#if has('road_distance') || has('road_name') || has('road_status')}
<section class="evidence-group" aria-label="Mapped roads">
  <h2>Mapped roads</h2>{@render status(roads)}
  {#if roads.status === 'ready'}
    {#if !roads.data.available}<p>No published road assessment</p>{:else}
      <dl>
        {#if has('road_name')}<dt>Nearest road name</dt><dd>{text(roads.data.ties[0]?.name ?? roads.data.ties[0]?.Name)}</dd>{/if}
        {#if has('road_distance')}<dt>Mapped road distance</dt><dd>{roads.data.distance_feet == null ? 'Distance not recorded' : roads.data.distance_feet.toFixed(0) + ' feet'}</dd>{/if}
      </dl>
      <details><summary>Road status, ties and source</summary>
        {#if has('road_status')}<p>Road status: {text(roads.data.ties[0]?.status ?? roads.data.ties[0]?.Status)}</p>{/if}
        {#if roads.data.ties.length > 1 && (has('road_name') || has('road_status'))}{#each roads.data.ties as tie}<p>{#if has('road_name')}{text(tie.name ?? tie.Name)}{/if}{#if has('road_name') && has('road_status')} · {/if}{#if has('road_status')}{text(tie.status ?? tie.Status)}{/if}</p>{/each}{/if}
        <p>{freshness(roads.data.freshness)} · Mapped proximity or status does not prove frontage, driveway, or legal access.</p>
      </details>
    {/if}
  {/if}
</section>
{/if}
{#if has('flood_unknown') || has('floodway_acres') || has('flood_zones')}
<section class="evidence-group" aria-label="FEMA flood mapping">
  <h2>FEMA flood mapping</h2>{@render status(flood)}
  {#if flood.status === 'ready'}
    {#if releaseId && flood.data.release?.id !== releaseId}<p>FEMA data changed. Refresh map to compare releases.</p>
    {:else if flood.data.status !== 'available'}<p>{flood.data.status === 'no_release' ? 'No FEMA release stored' : 'Parcel geometry unavailable'}.</p>
    {:else}
      <dl>
        {#if has('flood_unknown') && flood.data.acres.unknown > 0}<dt>Unknown coverage acreage</dt><dd class="warning">{acres(flood.data.acres.unknown)} acres</dd>{/if}
        {#if has('floodway_acres') && flood.data.acres.floodway > 0}<dt>Mapped floodway acreage</dt><dd class="warning">{acres(flood.data.acres.floodway)} acres</dd>{/if}
        {#if has('flood_zones')}<dt>Mapped flood zones</dt><dd>{flood.data.zones.map(z => `${z.code || 'unknown'} ${z.subtype || 'subtype not recorded'}: ${acres(z.acres)} acres`).join('; ') || 'No mapped zone overlap'}</dd>{/if}
      </dl>
      <details><summary>FEMA source and zero-area checks</summary>
        {#if has('flood_unknown')}<p>Coverage gap {acres(flood.data.acres.unknown)} acres.</p>{/if}
        {#if has('floodway_acres')}<p>Floodway {acres(flood.data.acres.floodway)} acres.</p>{/if}
        {#if has('flood_zones')}<p>Zone X does not distinguish minimal hazard from 0.2% annual-chance area.</p>{#each flood.data.panels as panel}<p>Panel {panel.number} · Effective {date(panel.effective_date)}</p>{/each}{/if}
        <p>FEMA archive {date(flood.data.release?.issued_at)} · Blank coverage is not proof of no flood risk. Zones may overlap.</p>
      </details>
    {/if}
  {/if}
</section>
{/if}
<style>
  .warning { color: #f4aa59; }
  .evidence-group { border: 1px solid var(--border); border-radius: 6px; margin: 10px 0; padding: 8px; }
  section { border-bottom: 1px solid var(--border); padding: 8px 0; overflow-wrap: anywhere; }
  h2 { font-size: 14px; font-weight: 650; }
  dl { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 6px 8px; font-size: 12px; line-height: 1.5; }
  dt { color: var(--muted, #bbb); }
  dd { margin: 0; text-align: right; }
  p, small { font-size: 12px; line-height: 1.5; }
  details { font-size: 12px; }
</style>

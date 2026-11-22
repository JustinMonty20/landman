<script lang="ts">
import { onMount } from 'svelte';
import { Tabs } from '@skeletonlabs/skeleton-svelte';
import ParcelMap, { jurisdictionColor, zoningColor } from '../lib/ParcelMap.svelte';
import FilterPanel from '../lib/FilterPanel.svelte';
import { emptyFilterDraft, filterSelections } from '../lib/filterDraft';
import { counties, type CountyId, type MapDetail } from '../lib/counties';
import { emptyShapes, floodStyles, mapLayerQuery, parcelShapes, readMapData, readMapLayer, viewportQuery, type FloodDetail, type FloodMap, type ParcelShape, type Viewport } from '../lib/flood';
import '../style.css';
let selectedId = $state<CountyId>('37179');
let detail = $state<MapDetail>('region');
let focusRequest = $state(0);
let panelOpen = $state(true);
let mapPanelOpen = $state(true);
let mapTab = $state('filters');
let draftFilters = $state(emptyFilterDraft());
const selected = $derived(counties.find((county) => county.id === selectedId)!);
const parcelView = $derived(detail !== 'region' && selected.status === 'configured');
let viewport = $state<Viewport>();
const layerQuery = $derived(parcelView && viewport ? mapLayerQuery(viewport) : '');
let parcels = $state<ParcelShape[]>([]);
let parcelError = $state('');
let parcelID = $state('');
let floodOn = $state(false);
type FloodCategory = keyof typeof floodStyles;
let floodCategory = $state<FloodCategory | ''>('');
let floodContact = $state<'area' | 'intersects'>('area');
const filterCount = $derived(filterSelections(draftFilters).length + (floodCategory ? 1 : 0));
let matches = $state<{ items: { id: string; parcel_number: string | null; address: string | null }[]; count: number; next_cursor?: string; source_stamp: string }>();
let matchError = $state('');
let matchCursor = $state('');
let cursorHistory = $state<string[]>([]);
function resetMatches() { matchCursor = ''; cursorHistory = []; }
let mapCount = $state<number | undefined>();
let mapStamp = $state<string>();
const floodParams = $derived(floodCategory ? new URLSearchParams({ flood_category: floodCategory, flood_contact: floodContact }) : undefined);
function clearFlood() { floodCategory = ''; floodContact = 'area'; resetMatches(); }

let flood = $state<FloodMap>();
let floodError = $state('');
let zoningOn = $state(false);
let roadsOn = $state(false);
let overlays = $state<Partial<Record<QualificationLayer, QualificationMap>>>({});
let overlayErrors = $state<Partial<Record<QualificationLayer, string>>>({});
type QualificationLayer = 'zoning' | 'roads';
type QualificationMap = import('geojson').FeatureCollection & {
  generation: { id: string; fetched_at: string; refresh_failed_at: string | null } | null;
  overview: boolean;
};
type ZoningSelection = { generation_id: string | null; districts: { jurisdiction: string; code: string; description: string; case_reference: string; source_date: string }[] };
let zoningPoint = $state<{ lon: number; lat: number }>();
let zoningSelection = $state<ZoningSelection>();
let zoningSelectionError = $state('');
const qualificationLayers = [
  { id: 'zoning', label: 'Zoning and jurisdiction', overviewNote: 'Overview: larger zoning areas only.' },
  { id: 'roads', label: 'Roads', overviewNote: 'Overview: main roads only.' }
] as const;

function loadQualification(layer: QualificationLayer, enabled: boolean, query: string) {
  overlayErrors[layer] = '';
  if (!enabled || !query) {
    overlays[layer] = undefined;
    return;
  }
  const task = new AbortController();
  readMapLayer<QualificationMap>(`/api/v1/qualification/map?layer=${layer}&${query}`, task.signal).then((data) => {
    if (!task.signal.aborted) overlays[layer] = data;
  }).catch((error: Error) => {
    if (task.signal.aborted) return;
    overlays[layer] = undefined;
    overlayErrors[layer] = error.message === '413' ? 'This layer exceeds the map display limit.' : 'Layer data is unavailable in this view.';
  });
  return () => task.abort();
}

$effect(() => loadQualification('zoning', zoningOn, layerQuery));
$effect(() => loadQualification('roads', roadsOn, layerQuery));

function inspectZoning(point: { lon: number; lat: number }) {
  if (!parcelView || !zoningOn) return;
  zoningPoint = point;
  mapTab = 'layers';
  mapPanelOpen = true;
}
$effect(() => { if (!parcelView || !zoningOn) zoningPoint = undefined; });
$effect(() => {
  const point = zoningPoint;
  const generation = overlays.zoning?.generation?.id;
  zoningSelection = undefined;
  zoningSelectionError = '';
  if (!point || !generation) return;
  const task = new AbortController();
  const params = new URLSearchParams({ lon: String(point.lon), lat: String(point.lat) });
  readMapData<ZoningSelection>(`/api/v1/qualification/zoning?${params}`, task.signal).then((data) => {
    if (task.signal.aborted) return;
    if (data.generation_id === generation) zoningSelection = data;
    else zoningSelectionError = 'Zoning data changed. Refresh the map and select the area again.';
  }).catch(() => {
    if (!task.signal.aborted) zoningSelectionError = 'Zoning details are unavailable.';
  });
  return () => task.abort();
});
let evidence = $state<FloodDetail>();
let detailError = $state('');
let parcelGeometry = $derived(parcelShapes(parcels));
const floodGeometry = $derived(!floodOn || !flood ? emptyShapes : floodCategory
  ? { ...flood, features: flood.features.filter((feature) => feature.properties?.category === floodCategory) }
  : flood);
const acreage = (value: number) => value > 0 && value < 0.01 ? '<0.01' : value.toFixed(2);
const releaseDate = (value: string) => value.slice(0, 10);

function inspectParcel(id: string) {
  parcelID = id;
  panelOpen = true;
}

$effect(() => {
  const view = viewport;
  parcels = [];
  mapCount = undefined;
  mapStamp = undefined;
  parcelError = '';
  if (!parcelView || !view || (view.zoom < 13 && !floodParams)) return;
  const task = new AbortController();
  const params = viewportQuery(view);
  if (floodParams) for (const [key, value] of floodParams) params.set(key, value);
  readMapData<{ mode: string; features?: ParcelShape[]; match_count: number; source_stamp: string }>(`/api/v1/parcels/map?${params}`, task.signal).then((data) => {
    if (!task.signal.aborted) { parcels = data.features ?? []; mapCount = data.match_count; mapStamp = data.source_stamp; }
  }).catch((error: Error) => {
    if (!task.signal.aborted) parcelError = error.message === '413' ? 'Zoom in to see individual parcels.' : 'Parcel boundaries are unavailable.';
  });
  return () => task.abort();
});

$effect(() => {
  floodError = '';
  if (!floodOn || !layerQuery) {
    flood = undefined;
    return;
  }
  const task = new AbortController();
  readMapLayer<FloodMap>(`/api/v1/flood/map?${layerQuery}${floodCategory ? `&flood_category=${floodCategory}` : ''}`, task.signal).then((data) => {
    if (!task.signal.aborted) flood = data;
  }).catch((error: Error) => {
    if (task.signal.aborted) return;
    flood = undefined;
    floodError = error.message === '413' ? 'Flood zones exceed the map display limit.' : 'FEMA flood zones are unavailable in this view.';
  });
  return () => task.abort();
});

$effect(() => {
 const params = floodParams;
 const cursor = matchCursor;
 matches = undefined;
 matchError = '';
 if (!parcelView || !params) return;
 const task = new AbortController();
 const query = new URLSearchParams(params);
 if (cursor) query.set('cursor', cursor);
 readMapData<NonNullable<typeof matches>>(`/api/v1/parcels?${query}`, task.signal).then(data => {
  if (!task.signal.aborted) matches = data;
 }).catch(() => { if (!task.signal.aborted) matchError = 'Matching parcels are unavailable.'; });
 return () => task.abort();
});
$effect(() => {
  const id = parcelID;
  const currentRelease = flood?.release?.id;
  evidence = undefined;
  detailError = '';
  if (!parcelView || !id) return;
  const task = new AbortController();
  readMapData<FloodDetail>(`/api/v1/parcels/${encodeURIComponent(id)}/flood`, task.signal).then((data) => {
    if (task.signal.aborted) return;
    if (currentRelease && currentRelease !== data.release?.id) {
      detailError = 'FEMA data changed. Move the map to load the current release.';
      return;
    }
    evidence = data;
  }).catch(() => {
    if (!task.signal.aborted) detailError = 'Flood evidence is unavailable.';
  });
  return () => task.abort();
});
function showSelectedFilters() {
  mapTab = 'filters';
  mapPanelOpen = true;
  if (detail === 'region') selectCounty(selectedId);
}
function selectCounty(id: CountyId) {
  if (selectedId !== id) parcelID = '';
  selectedId = id;
  focusRequest += 1;
  detail = 'county';
  panelOpen = true;
}
onMount(() => {
  const county = counties.find((county) => county.id === new URL(window.location.href).searchParams.get('county'));
  if (county) selectCounty(county.id);
});
</script>
{#snippet countySummary()}
  <button class="county-summary" aria-label="Show selected filters" onclick={showSelectedFilters}>{selected.name} · {filterCount} {filterCount === 1 ? 'filter' : 'filters'}</button>
{/snippet}

<svelte:head>
  <title>Landman · County discovery</title>
  <meta name="description" content="Select a county, then inspect its parcels at closer map levels." />
</svelte:head>
<div class="app">
  <header><a class="brand" href="/">▧ <span>landman</span></a></header>
  <main aria-label="Parcel discovery">
    <section class="map-area" aria-label="County map"><ParcelMap selected={selectedId} {focusRequest} onselect={selectCounty} ondetail={(value) => detail = value} onviewport={(value) => viewport = value} onparcel={inspectParcel} onzoning={inspectZoning} parcels={parcelGeometry} flood={floodGeometry} zoning={overlays.zoning ?? emptyShapes} roads={overlays.roads ?? emptyShapes} {parcelID} /></section>
    {#if !parcelView && filterCount > 0 && selected.status === 'configured'}{@render countySummary()}{/if}
    {#if selected.status === 'locked'}<p class="locked-notice" role="status">{selected.name} County is locked. Parcel discovery is not available.</p>{/if}
    {#if parcelView}
      <div class="map-controls">
        <div class="search-row"><label class="search"><span>Find parcel ID or address</span><input class="input" type="search" aria-describedby="parcel-availability" placeholder={`Find parcel in ${selected.name} County`} disabled /></label></div>
        <span class="sr-only" id="parcel-availability">Parcel search and filters are unavailable. Zoom in and select a parcel to see flood evidence.</span>
        {#if filterCount > 0}{@render countySummary()}{/if}
      </div>
    {/if}
    <div class="panels" style:top="112px">
      {#if parcelView}<button class="btn-icon preset-outlined-surface-300-700 bg-surface-900 edge-toggle left-toggle" class:open={mapPanelOpen} aria-label={mapPanelOpen ? 'Hide map panel' : 'Show map panel'} aria-expanded={mapPanelOpen} aria-controls="map-panel" onclick={() => mapPanelOpen = !mapPanelOpen}>{mapPanelOpen ? '‹' : '›'}</button>{/if}
      <div id="map-panel" role="region" aria-label="Map panel" class="left-panels" class:collapsed={!parcelView || !mapPanelOpen} inert={!parcelView || !mapPanelOpen} aria-hidden={!parcelView || !mapPanelOpen}>
        <Tabs class="filter-panel map-panel" value={mapTab} onValueChange={({ value }) => mapTab = value}>
          <Tabs.List class="map-tabs" aria-label="Map controls">
            <Tabs.Trigger class="btn" value="filters">Filters</Tabs.Trigger>
            <Tabs.Trigger class="btn" value="layers">Layers</Tabs.Trigger>
          </Tabs.List>
          <Tabs.Content class="map-tab-content" value="filters">
            <FilterPanel county={selected.name} bind:filters={draftFilters} />
<div class="flood-legend" aria-label="Flood parcel filter"><strong>FEMA flood parcel filter</strong>
<select class="select" aria-label="Flood category" bind:value={floodCategory} onchange={() => { resetMatches(); if (floodCategory) floodOn = true; }}>
<option value="">Any category (no flood filter)</option>
{#each Object.entries(floodStyles) as [key, style]}<option value={key}>{style.label}</option>{/each}</select>
{#if floodCategory}<label class="check"><input type="checkbox" class="checkbox" checked={floodContact === 'intersects'} onchange={(event) => { floodContact = event.currentTarget.checked ? 'intersects' : 'area'; resetMatches(); }} />Include boundary-only contacts</label><button class="btn btn-sm" onclick={clearFlood}>Clear flood filter</button>{/if}
</div>
          </Tabs.Content>
          <Tabs.Content class="map-tab-content layer-tab" value="layers">
            <div class="map-layers" role="group" aria-label="Map layers">
              <label class="check"><input class="checkbox" type="checkbox" bind:checked={zoningOn} /> Zoning and jurisdiction</label>
              <label class="check"><input class="checkbox" type="checkbox" bind:checked={roadsOn} /> Roads</label>
              <label class="check"><input class="checkbox" type="checkbox" bind:checked={floodOn} /> FEMA flood zones</label>
            </div>
            <div class="layer-legends">
              {#if floodOn}
                <div class="flood-legend">
                  {#each Object.entries(floodStyles) as [key, style]}
                    <span><i style:background-color={style.color} aria-hidden="true"></i> {style.label} <button class="btn btn-sm" aria-label={`Filter parcels by ${style.label}`} aria-pressed={floodCategory === key} onclick={() => { floodCategory = key as FloodCategory; floodOn = true; resetMatches(); }}>{floodCategory === key ? 'Selected' : 'Filter'}</button></span>
                  {/each}
                  {#if floodCategory}<label class="check"><input class="checkbox" type="checkbox" checked={floodContact === 'intersects'} onchange={(event) => { floodContact = event.currentTarget.checked ? 'intersects' : 'area'; resetMatches(); }} />Include boundary-only contacts</label><button class="btn btn-sm" onclick={clearFlood}>Clear flood filter</button>{/if}
                  <small>Source: FEMA National Flood Hazard Layer. Blank areas can have unknown coverage.</small>
                  {#if flood?.overview && flood.release}<small>Overview: simplified flood boundaries.</small>{/if}
                  {#if floodError}<p role="status">{floodError}</p>{:else if flood?.release}<small>Archive issue date: {releaseDate(flood.release.issued_at)}</small>{:else if flood}<p role="status">No FEMA release is stored.</p>{/if}
                </div>
              {/if}
              {#each qualificationLayers as layer}
                {#if layer.id === 'zoning' ? zoningOn : roadsOn}
                  <div class="flood-legend">
                    <span class:road-key={layer.id === 'roads'} class:zone-key={layer.id === 'zoning'}>{layer.label}</span>
                    {#if layer.id === 'zoning'}
                      <ul class="zoning-legend" aria-label="Zoning and jurisdiction legend">
                        <li><span class="zoning-swatch" style:background-color={zoningColor} aria-hidden="true"></span> Zoning area</li>
                        <li><span class="jurisdiction-swatch" style:border-color={jurisdictionColor} aria-hidden="true"></span> Municipality or ETJ boundary</li>
                      </ul>
                      <small>ETJ: extraterritorial jurisdiction. It is outside municipal limits.</small>
                      <small>Select a shaded zoning area to see its district codes.</small>
                      {#if zoningPoint}
                        <div class="selected-zoning" aria-live="polite">
                          <strong>Zoning at selected location</strong>
                          {#if zoningSelectionError}<p>{zoningSelectionError}</p>
                          {:else if !zoningSelection}<p>Loading zoning details…</p>
                          {:else if !zoningSelection.districts.length}<p>No zoning district is recorded at this point.</p>
                          {:else}
                            <ul aria-label="Zoning districts at selected location">
                              {#each zoningSelection.districts as district}
                                <li>
                                  <strong>{district.code || 'District code not recorded'}</strong> · {district.jurisdiction}
                                  {#if district.description}<p>Source description: {district.description}</p>{/if}
                                  {#if district.case_reference}<p>Case reference: {district.case_reference}</p>{/if}
                                  {#if district.source_date}<p>Source record date: {district.source_date}</p>{/if}
                                </li>
                              {/each}
                            </ul>
                            <p>Check the local zoning ordinance for permitted uses and building limits. If there is a case reference, use it to check for site-specific conditions.</p>
                          {/if}
                        </div>
                      {/if}
                    {/if}
                    <small>Source: Union County GIS</small>
                    {#if overlayErrors[layer.id]}
                      <p role="status">{overlayErrors[layer.id]}</p>
                    {:else if overlays[layer.id]?.generation}
                      {#if overlays[layer.id]?.overview}<small>{layer.overviewNote}</small>{/if}
                      <small>Retrieved: {releaseDate(overlays[layer.id]!.generation!.fetched_at)}</small>
                      {#if overlays[layer.id]!.generation!.refresh_failed_at}<p role="status">Last refresh failed. Showing stored data.</p>{/if}
                    {:else if overlays[layer.id]}
                      <p role="status">No data is stored for this layer.</p>
                    {/if}
                  </div>
                {/if}
              {/each}
            </div>
          </Tabs.Content>
        </Tabs>
      </div>
      {#if parcelView}
        <button class="btn-icon preset-outlined-surface-300-700 bg-surface-900 edge-toggle right-toggle" class:open={panelOpen} aria-label={panelOpen ? 'Hide matches' : 'Show matches'} aria-expanded={panelOpen} aria-controls="parcel-panel" onclick={() => panelOpen = !panelOpen}>{panelOpen ? '›' : '‹'}</button>
        <div class="right-panel" class:collapsed={!panelOpen} inert={!panelOpen} aria-hidden={!panelOpen}>
          {#if parcelID}
          <section class="results" id="parcel-panel" aria-label="Parcel flood evidence">
            <div class="results-heading">
              <button class="btn btn-sm" aria-label="Close parcel details" onclick={() => parcelID = ''}>×</button>
              <h1>Parcel flood evidence</h1>
              {#if evidence}<p>Parcel {evidence.parcel_number}</p>{/if}
            </div>
            <div class="flood-details" aria-live="polite">
              {#if detailError}<p>{detailError}</p>
              {:else if !evidence}<p>Loading flood evidence…</p>
              {:else if evidence.status === 'no_release'}<p>No FEMA release is stored.</p>
              {:else if evidence.status === 'missing_geometry'}<p>Parcel geometry is unavailable.</p>
              {:else if evidence.status === 'available'}
                <p>FEMA archive issue date: {releaseDate(evidence.release!.issued_at)}</p>
                <p>Unknown coverage: {acreage(evidence.acres.unknown)} acres of {acreage(evidence.acres.parcel)} acres</p>
                <p>Mapped floodway: {acreage(evidence.acres.floodway)} acres</p>
                <h2>Mapped zones</h2>
                {#if evidence.zones.length}
                  <ul>{#each evidence.zones as zone}<li>Zone {zone.code || 'unknown'}{zone.subtype ? ` · ${zone.subtype}` : ''}: {acreage(zone.acres)} acres</li>{/each}</ul>
                {:else}<p>No mapped zone overlaps this parcel.</p>{/if}
                <h2>FEMA panels</h2>
                {#if evidence.panels.length}<ul>{#each evidence.panels as panel}<li>{panel.number} · {panel.effective_date ?? 'Effective date unavailable'}</li>{/each}</ul>
                {:else}<p>No panel overlaps this parcel.</p>{/if}
                <p>Zones can overlap. Do not add their areas. A blank area is not proof that the parcel is free from flood risk.</p>
              {/if}
            </div>
          </section>
          {:else}
          <section class="results" id="parcel-panel" aria-label="Matching parcels">
            <div class="results-heading">
              <h1>{floodCategory ? 'Matching parcels' : 'Visible parcels'}</h1>
              <p>{#if floodCategory}{selected.name} County · {mapStamp && matches && mapStamp !== matches.source_stamp ? 'Data changed. Refresh map to compare counts.' : `${matches?.count ?? '…'} matches`}{#if mapCount && (!matches || !mapStamp || mapStamp === matches.source_stamp) !== undefined} · {mapCount} in map view{/if}{:else}{selected.name} County · Select a parcel to inspect flood evidence.{/if}</p>
              <label class="list-mode">Show<select class="select field-sm" disabled><option>All county matches</option><option>Matches in map view</option></select></label>
            </div>
            <div class="empty"><p>{matchError || parcelError || (floodCategory ? (!matches ? 'Loading matches…' : matches.count ? 'County matches' : 'No matching parcels.') : parcels.length ? 'Select a visible parcel for flood evidence.' : 'Zoom in to see parcel boundaries.')}</p>
              {#if floodCategory && matches?.items.length}<ul aria-label="Matching parcels">{#each matches.items as parcel (parcel.id)}<li><button class="btn btn-sm" onclick={() => inspectParcel(parcel.id)}>{parcel.parcel_number ?? 'Parcel ' + parcel.id.slice(0, 8)}{parcel.address ? ` · ${parcel.address}` : ''}</button></li>{/each}</ul>{:else if !floodCategory && parcels.length}<ul aria-label="Visible parcels">{#each parcels as parcel (parcel.id)}<li><button class="btn btn-sm" onclick={() => inspectParcel(parcel.id)}>{parcel.parcel_number ?? 'Parcel ' + parcel.id.slice(0, 8)}</button></li>{/each}</ul>{/if}
            </div>
            <nav class="pagination" aria-label="Result pages"><button class="btn btn-sm" disabled={!cursorHistory.length} onclick={() => { matchCursor = cursorHistory.at(-1) ?? ''; cursorHistory = cursorHistory.slice(0, -1); }}>Previous</button><span>Page 1 of 1</span><button class="btn btn-sm" disabled={!matches?.next_cursor} onclick={() => { cursorHistory = [...cursorHistory, matchCursor]; matchCursor = matches?.next_cursor ?? ''; }}>Next</button></nav>
          </section>
          {/if}
        </div>
      {/if}
    </div>
    <a class="coverage-link" href={`/coverage?county=${selectedId}`}>County coverage</a>
  </main>
</div>
<style>
h1 { font-size: 19px; font-weight: 650; }
.results .pagination { margin-top: auto; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
.map-controls { max-width: 440px; right: 330px; }
.county-summary { position: absolute; top: 12px; left: 12px; right: 65px; width: fit-content; max-width: calc(100% - 77px); background: var(--surface); border: 1px solid var(--border); border-radius: 4px; padding: 6px 10px; font-size: 11px; cursor: pointer; }
.map-controls .county-summary { position: static; max-width: 100%; }
.locked-notice { position: absolute; top: 12px; left: 12px; right: 65px; width: fit-content; max-width: 440px; font-size: 12px; padding: 8px 10px; background: var(--surface); border: 1px solid var(--border); border-radius: 4px; }
:global(.filter-panel.map-panel) { display: flex; flex-direction: column; height: 100%; padding: 0; overflow: hidden; }
:global(.map-tabs) { display: flex; flex-shrink: 0; border-bottom: 1px solid var(--border); }
:global(.map-tabs .btn) { flex: 1; border-bottom: 2px solid transparent; border-radius: 0; }
:global(.map-tabs .btn[data-selected]) { border-color: var(--accent); color: var(--accent); }
:global(.map-tabs .btn:focus-visible) { outline-offset: -3px; }
@media (prefers-reduced-motion: reduce) { :global(.map-tabs .btn) { transition: none; } }
:global(.map-tab-content) { flex: 1; min-height: 0; }
:global(.layer-tab) { overflow-y: auto; overscroll-behavior: contain; padding: 8px 16px 16px; }
.map-layers label { cursor: pointer; }
.layer-legends { display: grid; gap: 6px; }
.flood-legend { border: 1px solid var(--border); border-radius: 4px; padding: 6px 10px; font-size: 12px; }
.zone-key { color: #c7da93; }
.zoning-legend { display: grid; gap: 4px; }
.zoning-legend li { display: flex; align-items: center; gap: 6px; }
.zoning-swatch { width: 1.2em; height: .8em; opacity: .65; }
.jurisdiction-swatch { width: 1.2em; border-top: 2px dashed; }
.road-key { color: #ff996b; }
.flood-legend { display: grid; gap: 4px; width: fit-content; }
.flood-legend i { display: inline-block; width: .8em; height: .8em; }
.selected-zoning { border-top: 1px solid var(--border); padding-top: 6px; }
.selected-zoning li { margin-top: 4px; }
.flood-details, .empty { overflow: auto; padding: 10px; }
.flood-details h2 { font-weight: 650; }
.flood-details ul { padding-left: 1em; list-style: disc; }
.coverage-link { position: absolute; bottom: 12px; right: 12px; padding: 8px 12px; background: var(--surface); border: 1px solid var(--border); border-radius: 4px; font-size: 12px; color: #93c5fd; }
:global(input:disabled), :global(select:disabled), :global(button:disabled) { opacity: .6; cursor: not-allowed; }
@media (max-width: 900px) { .map-controls { right: 65px; max-width: 440px; } }
</style>

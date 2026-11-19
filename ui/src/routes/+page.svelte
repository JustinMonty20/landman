<script lang="ts">
import { onMount } from 'svelte';
import ParcelMap from '../lib/ParcelMap.svelte';
import FilterPanel from '../lib/FilterPanel.svelte';
import { emptyFilterDraft, filterSelections } from '../lib/filterDraft';
import { counties, type CountyId, type MapDetail } from '../lib/counties';
import { emptyShapes, parcelShapes, readMapData, viewportQuery, type FloodDetail, type FloodMap, type ParcelShape, type Viewport } from '../lib/flood';
import '../style.css';
let selectedId = $state<CountyId>('37179');
let detail = $state<MapDetail>('region');
let focusRequest = $state(0);
let panelOpen = $state(true);
let filtersOpen = $state(true);
let draftFilters = $state(emptyFilterDraft());
const selected = $derived(counties.find((county) => county.id === selectedId)!);
const parcelView = $derived(detail !== 'region' && selected.status === 'configured');
const filterCount = $derived(filterSelections(draftFilters).length);
let viewport = $state<Viewport>();
let parcels = $state<ParcelShape[]>([]);
let parcelError = $state('');
let parcelID = $state('');
let floodOn = $state(false);
let flood = $state<FloodMap>();
let floodError = $state('');
let evidence = $state<FloodDetail>();
let detailError = $state('');
let parcelGeometry = $derived(parcelShapes(parcels));
const acreage = (value: number) => value > 0 && value < 0.01 ? '<0.01' : value.toFixed(2);
const releaseDate = (value: string) => value.slice(0, 10);

function inspectParcel(id: string) {
  parcelID = id;
  panelOpen = true;
}

$effect(() => {
  const view = viewport;
  parcels = [];
  parcelError = '';
  if (!parcelView || !view || view.zoom < 13) return;
  const task = new AbortController();
  const params = viewportQuery(view);
  readMapData<{ features: ParcelShape[] }>(`/api/v1/parcels/map?${params}`, task.signal).then((data) => {
    if (!task.signal.aborted) parcels = data.features;
  }).catch((error: Error) => {
    if (!task.signal.aborted) parcelError = error.message === '413' ? 'Zoom in to see individual parcels.' : 'Parcel boundaries are unavailable.';
  });
  return () => task.abort();
});

$effect(() => {
  const view = viewport;
  flood = undefined;
  floodError = '';
  if (!parcelView || !floodOn || !view || view.zoom < 9) return;
  const task = new AbortController();
  const params = viewportQuery(view);
  readMapData<FloodMap>(`/api/v1/flood/map?${params}`, task.signal).then((data) => {
    if (!task.signal.aborted) flood = data;
  }).catch((error: Error) => {
    if (!task.signal.aborted) floodError = error.message === '413' ? 'Zoom in to see FEMA flood zones.' : 'FEMA flood zones are unavailable in this view.';
  });
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
  filtersOpen = true;
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
<svelte:head>
  <title>Landman · County discovery</title>
  <meta name="description" content="Select a county, then inspect its parcels at closer map levels." />
</svelte:head>
<div class="app">
  <header><a class="brand" href="/">▧ <span>landman</span></a></header>
  <main aria-label="Parcel discovery">
    <section class="map-area" aria-label="County map"><ParcelMap selected={selectedId} {focusRequest} onselect={selectCounty} ondetail={(value) => detail = value} onviewport={(value) => viewport = value} onparcel={inspectParcel} parcels={parcelGeometry} flood={flood ?? emptyShapes} {parcelID} /></section>
    {#if filterCount > 0 && selected.status === 'configured'}<button class="county-summary" class:with-search={parcelView} aria-label="Show selected filters" onclick={showSelectedFilters}>{selected.name} · {filterCount} {filterCount === 1 ? 'filter' : 'filters'}</button>{/if}
    {#if selected.status === 'locked'}<p class="locked-notice" role="status">{selected.name} County is locked. Parcel discovery is not available.</p>{/if}
    {#if parcelView}
      <div class="map-controls">
        <div class="search-row"><label class="search"><span>Find parcel ID or address</span><input class="input" type="search" aria-describedby="parcel-availability" placeholder={`Find parcel in ${selected.name} County`} disabled /></label></div>
        <span class="sr-only" id="parcel-availability">Parcel search and filters are unavailable. Zoom in and select a parcel to see flood evidence.</span>
        <label class="flood-toggle"><input type="checkbox" bind:checked={floodOn} /> FEMA flood zones</label>
        {#if floodOn}
          <div class="flood-legend">
            <span><i class="floodway"></i> Floodway</span>
            <span><i class="sfha"></i> Special flood hazard area</span>
            <span><i class="other"></i> Other mapped zones</span>
            <small>Source: FEMA National Flood Hazard Layer. Blank areas can have unknown coverage.</small>
            {#if floodError}<p role="status">{floodError}</p>{:else if flood?.release}<small>Archive issue date: {releaseDate(flood.release.issued_at)}</small>{:else if flood}<p role="status">No FEMA release is stored.</p>{/if}
          </div>
        {/if}
      </div>
    {/if}
    <div class="panels" style:top="112px">
      {#if parcelView}<button id="filter-toggle" class="btn-icon preset-outlined-surface-300-700 bg-surface-900 edge-toggle left-toggle" class:open={filtersOpen} aria-label={filtersOpen ? 'Hide filters' : 'Show filters'} aria-expanded={filtersOpen} aria-controls="filters" onclick={() => filtersOpen = !filtersOpen}>{filtersOpen ? '‹' : '›'}</button>{/if}
      <div class="left-panels" class:collapsed={!parcelView || !filtersOpen} inert={!parcelView || !filtersOpen} aria-hidden={!parcelView || !filtersOpen}>
        <FilterPanel county={selected.name} bind:filters={draftFilters} />
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
              <h1>Parcel results unavailable</h1>
              <p>{selected.name} County · Filter selections do not run a search.</p>
              <label class="list-mode">Show<select class="select field-sm" disabled><option>All county matches</option><option>Matches in map view</option></select></label>
            </div>
            <div class="empty"><p>{parcelError || (parcels.length ? 'Select a visible parcel for flood evidence.' : 'Zoom in to see parcel boundaries.')}</p>
              {#if parcels.length}<ul aria-label="Visible parcels">{#each parcels as parcel (parcel.id)}<li><button class="btn btn-sm" onclick={() => inspectParcel(parcel.id)}>{parcel.parcel_number ?? 'Parcel ' + parcel.id.slice(0, 8)}{parcel.address ? ` · ${parcel.address}` : ''}</button></li>{/each}</ul>{/if}
            </div>
            <nav class="pagination" aria-label="Result pages"><button class="btn btn-sm" disabled>Previous</button><span>Page 1 of 1</span><button class="btn btn-sm" disabled>Next</button></nav>
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
.county-summary.with-search { top: 64px; }
.locked-notice { position: absolute; top: 12px; left: 12px; right: 65px; width: fit-content; max-width: 440px; font-size: 12px; padding: 8px 10px; background: var(--surface); border: 1px solid var(--border); border-radius: 4px; }
.flood-toggle, .flood-legend { background: var(--surface); border: 1px solid var(--border); border-radius: 4px; padding: 6px 10px; font-size: 12px; }
.flood-toggle { display: inline-flex; gap: 6px; align-items: center; }
.flood-legend { display: grid; gap: 4px; width: fit-content; }
.flood-legend i { display: inline-block; width: .8em; height: .8em; }
.floodway { background: #dc78bf; }.sfha { background: #65cfe4; }.other { background: #8b9ee8; }
.flood-details, .empty { overflow: auto; padding: 10px; }
.flood-details h2 { font-weight: 650; }
.flood-details ul { padding-left: 1em; list-style: disc; }
.coverage-link { position: absolute; bottom: 12px; right: 12px; padding: 8px 12px; background: var(--surface); border: 1px solid var(--border); border-radius: 4px; font-size: 12px; color: #93c5fd; }
:global(input:disabled), :global(select:disabled), :global(button:disabled) { opacity: .6; cursor: not-allowed; }
@media (max-width: 900px) { .map-controls { right: 65px; max-width: 440px; } }
</style>

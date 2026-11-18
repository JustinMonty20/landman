<script lang="ts">
import { onMount } from 'svelte';
import ParcelMap from '../lib/ParcelMap.svelte';
import FilterPanel from '../lib/FilterPanel.svelte';
import { emptyFilterDraft, filterSelections } from '../lib/filterDraft';
import { counties, type CountyId, type MapDetail } from '../lib/counties';
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
function showSelectedFilters() {
  filtersOpen = true;
  if (detail === 'region') selectCounty(selectedId);
}
function selectCounty(id: CountyId) {
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
    <section class="map-area" aria-label="County map"><ParcelMap selected={selectedId} {focusRequest} onselect={selectCounty} ondetail={(value) => detail = value} /></section>
    {#if filterCount > 0 && selected.status === 'configured'}<button class="county-summary" class:with-search={parcelView} aria-label="Show selected filters" onclick={showSelectedFilters}>{selected.name} · {filterCount} {filterCount === 1 ? 'filter' : 'filters'}</button>{/if}
    {#if selected.status === 'locked'}<p class="locked-notice" role="status">{selected.name} County is locked. Parcel discovery is not available.</p>{/if}
    {#if parcelView}
      <div class="map-controls">
        <div class="search-row"><label class="search"><span>Find parcel ID or address</span><input class="input" type="search" aria-describedby="parcel-availability" placeholder={`Find parcel in ${selected.name} County`} disabled /></label></div>
        <span class="sr-only" id="parcel-availability">Parcel data is not connected yet. Search is unavailable. Filter selections do not run a search.</span>
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
          <section class="results" id="parcel-panel" aria-label="Matching parcels">
            <div class="results-heading">
              <h1>Parcel results unavailable</h1>
              <p>{selected.name} County · Parcel data is not connected yet. Filter selections do not run a search.</p>
              <label class="list-mode">Show<select class="select field-sm" disabled><option>All county matches</option><option>Matches in map view</option></select></label>
            </div>
            <div class="empty"><p>{detail === 'county' ? 'County match counts will appear here when parcel data is connected.' : 'Visible parcel boundaries will appear here when parcel data is connected.'}</p></div>
            <nav class="pagination" aria-label="Result pages"><button class="btn btn-sm" disabled>Previous</button><span>Page 1 of 1</span><button class="btn btn-sm" disabled>Next</button></nav>
          </section>
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
.coverage-link { position: absolute; bottom: 12px; right: 12px; padding: 8px 12px; background: var(--surface); border: 1px solid var(--border); border-radius: 4px; font-size: 12px; color: #93c5fd; }
:global(input:disabled), :global(select:disabled), :global(button:disabled) { opacity: .6; cursor: not-allowed; }
@media (max-width: 900px) { .map-controls { right: 65px; max-width: 440px; } }
</style>

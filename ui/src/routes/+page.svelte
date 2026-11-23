<script lang="ts">
import { onMount, tick } from 'svelte';
import { Tabs } from '@skeletonlabs/skeleton-svelte';
import ParcelEvidence from '../lib/ParcelEvidence.svelte';
import { detailGroups, detailFields, defaultDetailFields, parseDetailFields, type DetailField } from '../lib/detailGroups';
import ParcelMap, { jurisdictionColor, zoningColor } from '../lib/ParcelMap.svelte';
import FilterPanel from '../lib/FilterPanel.svelte';
import { emptyFilterDraft, filterSelections } from '../lib/filterDraft';
import { counties, type CountyId, type MapDetail } from '../lib/counties';
import { emptyShapes, floodStyles, mapLayerQuery, parcelShapes, readMapData, readMapLayer, viewportQuery, type FloodDetail, type FloodMap, type ParcelShape, type Viewport } from '../lib/flood';
import '../style.css';
let groups = $state<DetailField[]>([]);
let draftFields = $state<DetailField[]>(defaultDetailFields);
let preferenceStatus = $state('Loading saved details…');
let editingDetails = $state(false);
let savingDetails = $state(false);
function toggleField(group: DetailField) {
 draftFields = draftFields.includes(group) ? draftFields.filter(item => item !== group) : detailFields.filter(item => item.id === group || draftFields.includes(item.id)).map(item => item.id);
}
async function saveFields() {
 savingDetails = true;
 preferenceStatus = 'Saving…';
 try {
  const response = await fetch('/api/me/parcel-detail-fields', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ fields: draftFields }) });
  if (!response.ok) throw new Error();
  groups = parseDetailFields(await response.json());
  draftFields = [...groups];
  editingDetails = false;
  preferenceStatus = 'Detail fields saved.';
 } catch { preferenceStatus = 'Could not save detail fields. Try again.'; }
 finally { savingDetails = false; }
}
let selectedId = $state<CountyId>('37179');
let detail = $state<MapDetail>('region');
let focusRequest = $state(0);
let parcelFocus = $state<{ point: [number, number] }>();
let pendingParcelFocus = '';
let parcelFocusError = $state(false);
let panelOpen = $state(true);
let mapPanelOpen = $state(true);
let mapTab = $state('filters');
let evidenceTab = $state('evidence');
let research = $state<{ id: string; parcelNumber: string | null; coordinates: [number, number] | null }>();
function updateResearch(id: string, parcelNumber: string | null, coordinates: [number, number] | null) {
  if (id === parcelID) research = { id, parcelNumber, coordinates };
}
let draftFilters = $state(emptyFilterDraft());
const selected = $derived(counties.find((county) => county.id === selectedId)!);
const parcelView = $derived(detail !== 'region' && selected.status === 'configured');
let viewport = $state<Viewport>();
const layerQuery = $derived(parcelView && viewport ? mapLayerQuery(viewport) : '');
let parcels = $state<ParcelShape[]>([]);
let parcelError = $state('');
let parcelID = $state('');
const currentResearch = $derived(research?.id === parcelID ? research : undefined);
type ParcelSuggestion = { id: string; parcel_number: string | null; address: string | null };
let finderText = $state('');
let finderOpen = $state(false);
let suggestions = $state<ParcelSuggestion[]>([]);
let finderStatus = $state('');
let activeSuggestion = $state(0);
const finderQuery = $derived(finderText.trim());
$effect(() => {
  const query = finderQuery;
  const available = parcelView;
  const county = selectedId;
  suggestions = [];
  finderStatus = '';
  activeSuggestion = 0;
  if (!available || county !== '37179') return;
  const length = new TextEncoder().encode(query).length;
  if (length < 2) return;
  if (length > 80) { finderStatus = 'Enter 80 bytes or fewer.'; return; }
  const task = new AbortController();
  finderStatus = 'Searching…';
  readMapData<{ items: ParcelSuggestion[] }>(`/api/v1/parcels/lookup?q=${encodeURIComponent(query)}`, task.signal).then(data => {
    if (task.signal.aborted) return;
    suggestions = data.items;
    finderStatus = data.items.length ? '' : 'No parcels found.';
  }).catch(() => { if (!task.signal.aborted) finderStatus = 'Parcel search is unavailable. Try again.'; });
  return () => task.abort();
});
function chooseSuggestion(parcel: ParcelSuggestion) {
  finderOpen = false;
  finderText = '';
  inspectParcel(parcel.id, true);
}
function finderKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { finderOpen = false; return; }
  if (!suggestions.length) return;
  if (!finderOpen && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) finderOpen = true;
  if (!finderOpen) return;
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault();
    activeSuggestion = (activeSuggestion + (event.key === 'ArrowDown' ? 1 : suggestions.length - 1)) % suggestions.length;
    void tick().then(() => document.getElementById(`parcel-suggestion-${activeSuggestion}`)?.scrollIntoView({ block: 'nearest' }));
  }
  if (event.key === 'Enter') { event.preventDefault(); chooseSuggestion(suggestions[activeSuggestion]); }
}
let floodOn = $state(false);
type FloodCategory = keyof typeof floodStyles;
let floodCategory = $state<FloodCategory | ''>('');
let jurisdiction = $state('');
let jurisdictionMap = $state<QualificationMap>();
let jurisdictionError = $state(false);
let floodContact = $state<'area' | 'intersects'>('area');
const filterCount = $derived(filterSelections(draftFilters).length + (floodCategory ? 1 : 0) + (jurisdiction ? 1 : 0));
let soilChoices = $state<{key: string; name: string}[]>([]);
$effect(() => {
 if (!parcelView) return;
 const task = new AbortController();
 readMapData<{items: typeof soilChoices}>('/api/v1/parcels/soil-types', task.signal).then(data => { if (!task.signal.aborted) soilChoices = data.items; }).catch(() => {});
 return () => task.abort();
});
let matches = $state<{ items: { id: string; parcel_number: string | null; address: string | null }[]; count: number; next_cursor?: string; source_stamp: string }>();
let matchError = $state('');
let matchCursor = $state('');
let cursorHistory = $state<string[]>([]);
function resetMatches() { matchCursor = ''; cursorHistory = []; }
let mapCount = $state<number | undefined>();
let mapStamp = $state<string>();
const spatialParams = $derived.by(() => {
  const params = new URLSearchParams({ discovery: 'browse' });
  if (draftFilters.minAcres !== undefined && draftFilters.minAcres >= 0) params.set('min_acres', String(draftFilters.minAcres));
  if (draftFilters.maxAcres !== undefined && draftFilters.maxAcres >= 0) params.set('max_acres', String(draftFilters.maxAcres));
  if (draftFilters.businessName) params.set('entity_name_indicator', 'matched');
  if (draftFilters.outOfState) params.set('out_of_state_mailing', 'yes');
  if (draftFilters.delinquent) params.set('delinquent', 'true');
  if (draftFilters.foreclosure) params.set('foreclosure', 'true');
  if (draftFilters.vacancyTier) params.set('vacancy_tier', draftFilters.vacancyTier);
  if (draftFilters.soilType) params.set('soil_type', draftFilters.soilType);
  if (draftFilters.soilUse && draftFilters.soilRating) { params.set('soil_use', draftFilters.soilUse); params.set('soil_rating', draftFilters.soilRating); }
  if (floodCategory) { params.set('flood_category', floodCategory); params.set('flood_contact', floodContact); }
  if (jurisdiction) params.set('jurisdiction', jurisdiction);
  return params;
});
function selectJurisdiction(key: string) {
  jurisdiction = jurisdiction === key ? '' : key;
  zoningPoint = undefined;
  resetMatches();
}
function clearFlood() { floodCategory = ''; floodContact = 'area'; resetMatches(); }

let previousFilterQuery = "";
let previousViewport = '';
$effect(() => {
 const query = spatialParams.toString();
 const view = parcelView ? viewport?.bbox ?? '' : '';
 if (query !== previousFilterQuery || view !== previousViewport) {
  previousFilterQuery = query;
  previousViewport = view;
  resetMatches();
 }
});
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
const jurisdictionChoices = $derived((overlays.zoning?.features ?? []).flatMap(({ properties }) => {
  if (!properties || !['municipality_label', 'etj_label'].includes(properties.kind)) return [];
  const isETJ = properties.kind === 'etj_label';
  return [{ key: `${isETJ ? 'etj' : 'municipality'}:${properties.name}`, label: `${properties.name}${isETJ ? ' ETJ' : ''}` }];
}));
$effect(() => {
  jurisdictionMap = undefined;
  jurisdictionError = false;
  if (!parcelView || !zoningOn || !jurisdiction) return;
  const task = new AbortController();
  const params = new URLSearchParams({ layer: 'zoning', overview: 'true', jurisdiction });
  readMapData<QualificationMap>(`/api/v1/qualification/map?${params}`, task.signal).then((data) => {
    if (!task.signal.aborted) jurisdictionMap = data;
  }).catch(() => { if (!task.signal.aborted) jurisdictionError = true; });
  return () => task.abort();
});

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
let parcelGeometry = $derived(parcelShapes(parcels));
const floodGeometry = $derived(!floodOn || !flood ? emptyShapes : floodCategory
  ? { ...flood, features: flood.features.filter((feature) => feature.properties?.category === floodCategory) }
  : flood);
const acreage = (value: number) => value > 0 && value < 0.01 ? '<0.01' : value.toFixed(2);
const releaseDate = (value: string) => value.slice(0, 10);

function inspectParcel(id: string, focus = false) {
  pendingParcelFocus = focus ? id : '';
  parcelFocusError = false;
  if (parcelID !== id) research = undefined;
  parcelID = id;
  panelOpen = true;
}

function focusParcelFromList(id: string, point: [number, number] | null) {
  if (parcelID !== id || pendingParcelFocus !== id) return;
  pendingParcelFocus = '';
  if (point) parcelFocus = { point };
  else parcelFocusError = true;
}

$effect(() => {
  const view = viewport;
  parcels = [];
  mapCount = undefined;
  mapStamp = undefined;
  parcelError = '';
  if (!parcelView || !view) return;
  const task = new AbortController();
  const params = viewportQuery(view);
  if (spatialParams) for (const [key, value] of spatialParams) params.set(key, value);
  type ParcelMapData = { mode: string; features?: ParcelShape[]; match_count: number; source_stamp: string };
  const load = () => readMapData<ParcelMapData>(`/api/v1/parcels/map?${params}`, task.signal);
  load().catch(async (error: Error) => {
    if (error.message !== '413' || task.signal.aborted) throw error;
    params.set('zoom', '11');
    const summary = await load();
    if (!task.signal.aborted) parcelError = 'Too many parcels to draw. Zoom in or apply a filter.';
    return summary;
  }).then((data) => {
    if (!task.signal.aborted) { parcels = data.features ?? []; mapCount = data.match_count; mapStamp = data.source_stamp; }
  }).catch(() => {
    if (!task.signal.aborted) parcelError = 'Parcel boundaries are unavailable.';
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
 const params = spatialParams;
 const view = parcelView ? viewport?.bbox : undefined;
 const cursor = matchCursor;
 matches = undefined;
 matchError = '';
 if (!parcelView || !params) return;
 const task = new AbortController();
 const query = new URLSearchParams(params);
 if (view) query.set('viewport_bbox', view);
 if (cursor) query.set('cursor', cursor);
 readMapData<NonNullable<typeof matches>>(`/api/v1/parcels?${query}`, task.signal).then(data => {
  if (!task.signal.aborted) matches = data;
 }).catch(() => { if (!task.signal.aborted) matchError = 'Matching parcels are unavailable.'; });
 return () => task.abort();
});
function showSelectedFilters() {
  mapTab = 'filters';
  mapPanelOpen = true;
  if (detail === 'region') selectCounty(selectedId);
}
function selectCounty(id: CountyId) {
  if (selectedId !== id) { parcelID = ''; finderText = ''; finderOpen = false; }
  selectedId = id;
  focusRequest += 1;
  detail = 'county';
  panelOpen = true;
}
onMount(() => {
  const task = new AbortController();
  fetch('/api/me/parcel-detail-fields', { signal: task.signal }).then(async response => {
    if (!response.ok) throw new Error();
    groups = parseDetailFields(await response.json());
    draftFields = [...groups];
    preferenceStatus = '';
  }).catch(() => { if (!task.signal.aborted) { groups = [...defaultDetailFields]; draftFields = [...groups]; preferenceStatus = 'Saved details unavailable. Showing default fields; changes cannot be saved until reconnected.'; } });
  const county = counties.find((county) => county.id === new URL(window.location.href).searchParams.get('county'));
  if (county) selectCounty(county.id);
return () => task.abort();
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
    <section class="map-area" aria-label="County map"><ParcelMap selected={selectedId} {focusRequest} {parcelFocus} onselect={selectCounty} ondetail={(value) => detail = value} onviewport={(value) => viewport = value} onparcel={inspectParcel} onzoning={inspectZoning} parcels={parcelGeometry} flood={floodGeometry} zoning={(jurisdiction ? jurisdictionMap : overlays.zoning) ?? emptyShapes} roads={overlays.roads ?? emptyShapes} {parcelID} /></section>
    {#if !parcelView && filterCount > 0 && selected.status === 'configured'}{@render countySummary()}{/if}
    {#if selected.status === 'locked'}<p class="locked-notice" role="status">{selected.name} County is locked. Parcel discovery is not available.</p>{/if}
    {#if parcelView}
      <div class="map-controls">
        <div class="search-row" onfocusout={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) finderOpen = false; }}>
          <label class="search"><span>Find parcel ID or address</span><input class="input" type="search" role="combobox" aria-autocomplete="list" aria-expanded={finderOpen && !!finderQuery} aria-controls="parcel-suggestions" aria-activedescendant={finderOpen && suggestions.length ? `parcel-suggestion-${activeSuggestion}` : undefined} placeholder={`Find parcel in ${selected.name} County`} bind:value={finderText} onfocus={() => finderOpen = true} oninput={() => finderOpen = true} onkeydown={finderKeydown} /></label>
          {#if finderOpen && finderQuery}<div id="parcel-suggestions" class="search-suggestions" role="listbox" aria-label="Parcel suggestions">
            {#each suggestions as parcel, index (parcel.id)}<button id={`parcel-suggestion-${index}`} role="option" aria-selected={index === activeSuggestion} onclick={() => chooseSuggestion(parcel)}>{parcel.address || 'Address not recorded'} · {parcel.parcel_number || `Parcel ${parcel.id.slice(0, 8)}`}</button>{/each}
            {#if finderStatus}<p role="status">{finderStatus}</p>{/if}
          </div>{/if}
        </div>
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
            <FilterPanel county={selected.name} bind:filters={draftFilters} {soilChoices} />
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
                    <button class="btn btn-sm legend-item" aria-pressed={floodCategory === key} onclick={() => {
                      if (floodCategory === key) clearFlood();
                      else { floodCategory = key as FloodCategory; resetMatches(); }
                    }}><i style:background-color={style.color} aria-hidden="true"></i> {style.label}</button>
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
                      <small>Select a named boundary to filter parcels.</small>
                      <div class="jurisdiction-choices" role="group" aria-label="Jurisdiction parcel filters">
                        {#each jurisdictionChoices as choice (choice.key)}
                          <button class="btn btn-sm legend-item" aria-pressed={jurisdiction === choice.key} onclick={() => selectJurisdiction(choice.key)}>{choice.label}</button>
                        {/each}
                      </div>
                      {#if jurisdictionError}<p role="status">The selected jurisdiction boundary is unavailable.</p>{/if}
                      {#if !jurisdiction}<small>Select a shaded zoning area to see its district codes.</small>{/if}
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
          <section class="results" id="parcel-panel" aria-label="Parcel evidence">
            <div class="results-heading"><button class="btn btn-sm" aria-label="Close parcel details" onclick={() => parcelID = ''}>×</button><h1>Parcel evidence</h1></div>
            <Tabs value={evidenceTab} onValueChange={({ value }) => evidenceTab = value}>
              <Tabs.List class="map-tabs" aria-label="Parcel evidence views">
                <Tabs.Trigger class="btn" value="evidence">Evidence</Tabs.Trigger>
                <Tabs.Trigger class="btn" value="research">Research links</Tabs.Trigger>
              </Tabs.List>
              <Tabs.Content value="evidence">
            <div class="detail-editor"><button class="btn btn-sm" aria-expanded={editingDetails} onclick={() => { draftFields = [...groups]; editingDetails = !editingDetails; }}>Choose detail fields</button>
{#if editingDetails}<form onsubmit={(event) => { event.preventDefault(); saveFields(); }}><fieldset disabled={savingDetails}><legend>Show individual fields in parcel details</legend>{#each detailGroups as group}<div class="field-picker-group"><strong>{group.label}</strong>{#each group.fields as field}<label class="check"><input type="checkbox" class="checkbox" checked={draftFields.includes(field.id)} onchange={() => toggleField(field.id)} />{field.label}</label>{/each}</div>{/each}<button class="btn btn-sm" type="submit">Save details</button></fieldset></form>{/if}<p role="status" aria-live="polite">{preferenceStatus}</p></div>
<div class="flood-details" aria-live="polite">{#if parcelFocusError}<p role="status">Parcel location is unavailable.</p>{/if}<ParcelEvidence id={parcelID} fields={groups} releaseId={flood?.release?.id} onlocation={focusParcelFromList} onresearch={updateResearch} /></div>
              </Tabs.Content>
              <Tabs.Content value="research">
              <div class="research-content">
                <ul aria-label="External research sites" class="research-sites">
                  {#if currentResearch?.parcelNumber}
                    <li><a href={'https://property.spatialest.com/nc/union#/property/' + encodeURIComponent(currentResearch.parcelNumber)} target="_blank" rel="noopener noreferrer">Union County property records</a> · Open the selected parcel’s county property record.</li>
                  {:else}
                    <li>Union County property records · A county parcel number is required to open the property record.</li>
                  {/if}
                  {#if currentResearch?.parcelNumber}
                    <li><a href={'https://unionnc.devnetwedge.com/parcel/view/' + encodeURIComponent(currentResearch.parcelNumber) + '/2026'} target="_blank" rel="noopener noreferrer">Union County tax records</a> · Open the selected parcel’s 2026 tax record and payment information.</li>
                  {:else}
                    <li>Union County tax records · A county parcel number is required to open the tax record.</li>
                  {/if}
                  {#if currentResearch?.coordinates && currentResearch.coordinates.every(Number.isFinite)}
                    <li><a href={'https://earth.google.com/web/search/' + currentResearch.coordinates[1] + ',' + currentResearch.coordinates[0]} target="_blank" rel="noopener noreferrer">Open satellite view</a> · Google Earth at a point inside the stored parcel. Imagery does not show boundaries or guarantee currency.</li>
                  {/if}
                </ul>
                <p>County parcel number: {currentResearch ? currentResearch.parcelNumber || 'Not recorded' : 'Loading…'}. Both county links open this parcel directly. The tax link opens the 2026 record. An unavailable lookup does not mean the county has no record.</p>
              </div>
              </Tabs.Content>
            </Tabs>
          </section>
          {:else}
          <section class="results" id="parcel-panel" aria-label="Matching parcels">
            <div class="results-heading">
              <h1>Matching parcels</h1>
              {#if spatialParams}
                {#if jurisdiction}<button class="btn btn-sm" onclick={() => selectJurisdiction(jurisdiction)}>Clear jurisdiction filter</button>{/if}
                {#if matches}<p>{selected.name} County · {matches.count} {matches.count === 1 ? 'match' : 'matches'}</p>{/if}
                {#if matches && mapStamp && mapStamp !== matches.source_stamp}
                  <p>Data changed. Refresh the map to compare counts.</p>
                {:else if matches && mapCount !== undefined && mapStamp === matches.source_stamp}
                  <p>{mapCount} in map view</p>
                {/if}
              {:else}
                <p>{selected.name} County</p>
              {/if}
            </div>
            <div class="empty">
              {#if parcelError}<p role="status">{parcelError}</p>{/if}
              <p>{matchError || (!matches ? 'Loading matches…' : matches.count ? 'County matches' : 'No matching parcels.')}</p>
              {#if spatialParams && matches?.items.length}<ul aria-label="Matching parcels">{#each matches.items as parcel (parcel.id)}<li><button class="btn btn-sm" onclick={() => inspectParcel(parcel.id, true)}>{parcel.parcel_number ?? 'Parcel ' + parcel.id.slice(0, 8)}{parcel.address ? ` · ${parcel.address}` : ''}</button></li>{/each}</ul>{:else if !spatialParams && parcels.length}<ul aria-label="Visible parcels">{#each parcels as parcel (parcel.id)}<li><button class="btn btn-sm" onclick={() => inspectParcel(parcel.id)}>{parcel.parcel_number ?? 'Parcel ' + parcel.id.slice(0, 8)}</button></li>{/each}</ul>{/if}
            </div>
            {#if spatialParams}<nav class="pagination" aria-label="Result pages"><button class="btn btn-sm" disabled={!cursorHistory.length} onclick={() => { matchCursor = cursorHistory.at(-1) ?? ''; cursorHistory = cursorHistory.slice(0, -1); }}>Previous</button><span>Page {cursorHistory.length + 1}</span><button class="btn btn-sm" disabled={!matches?.next_cursor} onclick={() => { cursorHistory = [...cursorHistory, matchCursor]; matchCursor = matches?.next_cursor ?? ''; }}>Next</button></nav>{/if}
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
.jurisdiction-choices { display: grid; max-height: 12rem; overflow-y: auto; }
.legend-item { justify-content: flex-start; gap: 6px; text-align: left; white-space: normal; }
.legend-item[aria-pressed='true'] { background: var(--color-surface-700); box-shadow: inset 2px 0 var(--accent); }
.flood-legend i { flex-shrink: 0; display: inline-block; width: .8em; height: .8em; }
.selected-zoning { border-top: 1px solid var(--border); padding-top: 6px; }
.selected-zoning li { margin-top: 4px; }
.field-picker-group { display: grid; gap: 3px; margin: 7px 0; }
.research-content { padding: 12px 14px; font-size: 13px; line-height: 1.5; overflow-y: auto; }
.research-sites { list-style: disc; padding-left: 20px; margin-bottom: 14px; }
.research-sites li { margin-bottom: 12px; }
.research-sites a { color: #93c5fd; text-decoration: underline; }
.detail-editor { padding: 8px 10px; border-bottom: 1px solid var(--border); font-size: 12px; }
.detail-editor fieldset { display: grid; gap: 4px; }
.flood-details, .empty { overflow: auto; padding: 10px; }
.coverage-link { position: absolute; bottom: 12px; right: 12px; padding: 8px 12px; background: var(--surface); border: 1px solid var(--border); border-radius: 4px; font-size: 12px; color: #93c5fd; }
:global(input:disabled), :global(select:disabled), :global(button:disabled) { opacity: .6; cursor: not-allowed; }
@media (max-width: 900px) { .map-controls { right: 65px; max-width: 440px; } }
</style>

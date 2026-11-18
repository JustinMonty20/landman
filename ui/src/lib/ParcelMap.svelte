<script lang="ts">
import { onMount } from 'svelte';
import { Map, Marker, NavigationControl, setWorkerUrl } from 'maplibre-gl';
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url';
import 'maplibre-gl/dist/maplibre-gl.css';
import { counties, countyBounds, countyGeometry, mapDetail, type CountyId, type MapDetail } from './counties';

let { selected, focusRequest, onselect, ondetail }: {
  selected: CountyId;
  focusRequest: number;
  onselect: (id: CountyId) => void;
  ondetail: (detail: MapDetail) => void;
} = $props();
let container: HTMLDivElement;
let map: Map | undefined;
let ready = $state(false);
let failed = $state(false);
let lastFocusRequest = 0;
let markers: Marker[] = [];
const padding = { top: 110, bottom: 185, left: 35, right: 35 };

function reportDetail() {
  if (!map) return;
  const detail = mapDetail(map.getZoom());
  markers.forEach((marker) => { marker.getElement().hidden = detail === 'parcels'; });
  ondetail(detail);
}
function showRegion() {
  map?.fitBounds(countyBounds(), { padding, maxZoom: 8.5, duration: 0 });
}
function locateCounty() {
  if (!map) return;
  const county = counties.find((county) => county.id === selected)!;
  const camera = map.cameraForBounds(countyBounds(selected), { padding: { ...padding, right: county.status === 'configured' ? 355 : 35 }, maxZoom: 11 });
  if (camera) map.jumpTo({ ...camera, zoom: Math.max(9, camera.zoom ?? 9) });
}

onMount(() => {
  let observer: ResizeObserver | undefined;
  try {
    setWorkerUrl(workerUrl);
    map = new Map({
      container, bounds: countyBounds(), fitBoundsOptions: { padding, maxZoom: 8.5 },
      attributionControl: false,
      style: { version: 8, sources: {}, layers: [{ id: 'background', type: 'background', paint: { 'background-color': '#24272b' } }] }
    });
    map.addControl(new NavigationControl({ showCompass: false }), 'top-right');
    map.on('error', () => { failed = true; });
    map.on('load', () => {
      if (!map) return;
      map.addSource('counties', { type: 'geojson', data: countyGeometry });
      map.addLayer({ id: 'county-fill', type: 'fill', source: 'counties', paint: {
        'fill-color': ['match', ['get', 'status'], 'configured', '#35567d', '#303438'], 'fill-opacity': 0.5
      } });
      map.addLayer({ id: 'county-outline', type: 'line', source: 'counties', paint: { 'line-color': '#a3aab4', 'line-width': 1.5 } });
      map.addLayer({ id: 'county-selection', type: 'line', source: 'counties', filter: ['==', ['get', 'id'], selected], paint: { 'line-color': '#edba71', 'line-width': 3 } });
      for (const county of counties) {
        const label = document.createElement('button');
        label.type = 'button';
        label.className = 'county-map-label';
        label.textContent = `${county.name}\n${county.state}${county.status === 'locked' ? ' · Locked' : ''}`;
        label.setAttribute('aria-label', `Inspect ${county.name}, ${county.state}${county.status === 'locked' ? ': Locked' : ''}`);
        label.addEventListener('click', (event) => { event.stopPropagation(); onselect(county.id); });
        markers.push(new Marker({ element: label }).setLngLat([...county.center]).addTo(map));
      }
      map.on('click', 'county-fill', (event) => {
        const county = counties.find((county) => county.id === event.features?.[0]?.properties?.id);
        if (county) onselect(county.id);
      });
      map.on('mouseenter', 'county-fill', () => { if (map) map.getCanvas().style.cursor = 'pointer'; });
      map.on('mouseleave', 'county-fill', () => { if (map) map.getCanvas().style.cursor = ''; });
      ready = true;
      reportDetail();
    });
    map.on('moveend', reportDetail);
    observer = new ResizeObserver(() => map?.resize());
    observer.observe(container);
  } catch { failed = true; }
  return () => { observer?.disconnect(); markers.forEach((marker) => marker.remove()); map?.remove(); };
});

$effect(() => {
  if (ready && map) map.setFilter('county-selection', ['==', ['get', 'id'], selected]);
});
$effect(() => {
  if (!ready || focusRequest === lastFocusRequest) return;
  lastFocusRequest = focusRequest;
  locateCounty();
});
</script>
<div class="map-frame">
  <div class="canvas" bind:this={container} aria-label="County outline map"></div>
  <div class="map-actions">
    <button class="btn btn-sm preset-outlined-surface-300-700 bg-surface-900" disabled={!ready || failed} onclick={showRegion}>Show all counties</button>
    <button class="btn btn-sm preset-outlined-surface-300-700 bg-surface-900" disabled={!ready || failed} onclick={() => onselect(selected)}>Locate selected county</button>
    <div class="source">Boundaries: U.S. Census Bureau · Display only</div>
  </div>
  {#if failed}<p class="failure" role="status">Map unavailable. Open County coverage to select a county.</p>{/if}
</div>
<style>
.map-frame { position: relative; height: 100%; min-height: 380px; }
.canvas { position: absolute; inset: 0; }
.map-actions { position: absolute; bottom: 12px; left: 12px; right: 150px; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.failure, .source { background: var(--surface); padding: 10px; border: 1px solid var(--border); border-radius: 6px; font-size: 12px; pointer-events: none; }
.source { font-size: 10px; }
.failure { position: absolute; bottom: 155px; left: 12px; right: 12px; }
:global(.county-map-label) { color: #f0f2f5; background: #24272be8; border: 1px solid #727b88; border-radius: 4px; padding: 4px 7px; font-size: 11px; line-height: 1.3; white-space: pre; cursor: pointer; }
</style>

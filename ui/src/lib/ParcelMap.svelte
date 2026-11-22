<script module lang="ts">
  export const zoningColor = '#aebf77';
  export const jurisdictionColor = '#d5ce82';
</script>

<script lang="ts">
import { onMount } from 'svelte';
import { Map, Marker, NavigationControl, setWorkerUrl, type GeoJSONSource } from 'maplibre-gl';
import type { FeatureCollection } from 'geojson';
import { emptyShapes, floodStyles, type Viewport } from './flood';
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url';
import 'maplibre-gl/dist/maplibre-gl.css';
import { counties, countyBounds, countyGeometry, mapDetail, type CountyId, type MapDetail } from './counties';

let { selected, focusRequest, onselect, ondetail, onviewport, onparcel, onzoning, parcels, flood, zoning, roads, parcelID }: {
  selected: CountyId;
  focusRequest: number;
  onselect: (id: CountyId) => void;
  ondetail: (detail: MapDetail) => void;
  onviewport: (viewport: Viewport) => void;
  onparcel: (id: string) => void;
  onzoning: (point: { lon: number; lat: number }) => void;
  parcels: FeatureCollection;
  flood: FeatureCollection;
  zoning: FeatureCollection;
  roads: FeatureCollection;
  parcelID: string;
} = $props();
let container: HTMLDivElement;
let map: Map | undefined;
let ready = $state(false);
let failed = $state(false);
let lastFocusRequest = 0;
let markers: Marker[] = [];
let layerMarkers: Marker[] = [];

function placeMapLabels() {
  if (!map) return;
  const placed: { x: number; y: number; w: number; h: number }[] = [];
  for (const marker of layerMarkers) {
    const { x, y } = map.project(marker.getLngLat());
    const label = marker.getElement();
    const w = label.offsetWidth / 2 + 3;
    const h = label.offsetHeight / 2 + 2;
    const visible = x >= 0 && y >= 0 && x < container.clientWidth && y < container.clientHeight &&
      !placed.some((box) => Math.abs(x - box.x) < w + box.w && Math.abs(y - box.y) < h + box.h);
    label.style.visibility = visible ? '' : 'hidden';
    if (visible) placed.push({ x, y, w, h });
  }
}
const padding = { top: 110, bottom: 185, left: 35, right: 35 };

function reportDetail() {
  if (!map) return;
  const detail = mapDetail(map.getZoom());
  markers.forEach((marker) => { marker.getElement().hidden = detail === 'parcels'; });
  ondetail(detail);
  placeMapLabels();
  const bounds = map.getBounds();
  onviewport({ bbox: [bounds.getWest(), bounds.getSouth(), bounds.getEast(), bounds.getNorth()].map((n) => n.toFixed(6)).join(','), zoom: Math.floor(map.getZoom()) });
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
      map.addSource('zoning', { type: 'geojson', data: emptyShapes });
      map.addLayer({ id: 'zoning-fill', type: 'fill', source: 'zoning', filter: ['==', ['get', 'kind'], 'zoning'], paint: { 'fill-color': zoningColor, 'fill-opacity': 0.3, 'fill-outline-color': zoningColor } });
      map.addLayer({ id: 'jurisdiction-outline', type: 'line', source: 'zoning', filter: ['==', ['get', 'kind'], 'jurisdiction'], paint: { 'line-color': jurisdictionColor, 'line-width': 2, 'line-dasharray': [3, 2] } });
      map.addSource('flood-zones', { type: 'geojson', data: emptyShapes });
      map.addLayer({ id: 'flood-fill', type: 'fill', source: 'flood-zones', paint: {
        'fill-color': ['match', ['get', 'category'], 'floodway', floodStyles.floodway.color, 'sfha', floodStyles.sfha.color, 'moderate', floodStyles.moderate.color, 'minimal', floodStyles.minimal.color, 'unclassified', floodStyles.unclassified.color, floodStyles.unclassified.color], 'fill-opacity': 0.42
      } });
      map.addSource('roads', { type: 'geojson', data: emptyShapes });
      map.addLayer({ id: 'roads', type: 'line', source: 'roads', paint: { 'line-color': '#ff996b', 'line-width': 2 } });
      map.addSource('parcels', { type: 'geojson', data: emptyShapes });
      map.addLayer({ id: 'parcel-hit', type: 'fill', source: 'parcels', paint: { 'fill-opacity': 0 } });
      map.addLayer({ id: 'parcel-outline', type: 'line', source: 'parcels', paint: { 'line-color': '#c4cbd2', 'line-width': 1 } });
      map.addLayer({ id: 'parcel-selected', type: 'line', source: 'parcels', filter: ['==', ['get', 'id'], ''], paint: { 'line-color': '#edba71', 'line-width': 3 } });
      map.on('click', (event) => {
        if (!map) return;
        const hit = map.queryRenderedFeatures(event.point, { layers: ['parcel-hit'] })[0];
        if (hit) onparcel(String(hit.properties.id));
        const zoningHit = map.queryRenderedFeatures(event.point, { layers: ['zoning-fill'] })[0];
        if (zoningHit) {
          const { lng, lat } = event.lngLat.wrap();
          onzoning({ lon: lng, lat });
        }
        if (hit || zoningHit) return;
        if (mapDetail(map.getZoom()) === 'parcels') return;
        const county = counties.find((county) => county.id === map?.queryRenderedFeatures(event.point, { layers: ['county-fill'] })[0]?.properties?.id);
        if (county) onselect(county.id);
      });
      map.on('mouseenter', 'county-fill', () => { if (map) map.getCanvas().style.cursor = 'pointer'; });
      map.on('mouseleave', 'county-fill', () => { if (map) map.getCanvas().style.cursor = ''; });
      map.on('mouseenter', 'parcel-hit', () => { if (map) map.getCanvas().style.cursor = 'pointer'; });
      map.on('mouseleave', 'parcel-hit', () => { if (map) map.getCanvas().style.cursor = ''; });
      ready = true;
      reportDetail();
    });
    map.on('moveend', reportDetail);
    observer = new ResizeObserver(() => map?.resize());
    observer.observe(container);
  } catch { failed = true; }
  return () => { observer?.disconnect(); [...markers, ...layerMarkers].forEach((marker) => marker.remove()); map?.remove(); };
});

$effect(() => {
  if (ready && map) map.setFilter('county-selection', ['==', ['get', 'id'], selected]);
});
$effect(() => {
  if (!ready || !map) return;
  (map.getSource('parcels') as GeoJSONSource).setData(parcels);
  (map.getSource('flood-zones') as GeoJSONSource).setData(flood);
  map.setFilter('parcel-selected', ['==', ['get', 'id'], parcelID]);
});
$effect(() => {
  if (!ready || !map) return;
  (map.getSource('zoning') as GeoJSONSource).setData(zoning);
  (map.getSource('roads') as GeoJSONSource).setData(roads);
  layerMarkers.forEach((marker) => marker.remove());
  layerMarkers = [];
  for (const { geometry, properties } of [...zoning.features, ...roads.features]) {
    if (geometry.type !== 'Point' || typeof properties?.name !== 'string') continue;
    const label = document.createElement('span');
    label.className = properties.kind === 'road_label' ? 'layer-map-label road-map-label' : 'layer-map-label';
    label.textContent = properties.kind === 'etj_label' ? `${properties.name} ETJ` : properties.name;
    layerMarkers.push(new Marker({ element: label }).setLngLat([geometry.coordinates[0], geometry.coordinates[1]]).addTo(map));
  }
  placeMapLabels();
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
:global(.layer-map-label) { color: #f3e9a8; background: #24272be8; padding: 1px 4px; border-radius: 3px; font-size: 11px; white-space: nowrap; pointer-events: none; }
:global(.road-map-label) { color: #ffb691; font-size: 10px; }
:global(.county-map-label) { color: #f0f2f5; background: #24272be8; border: 1px solid #727b88; border-radius: 4px; padding: 4px 7px; font-size: 11px; line-height: 1.3; white-space: pre; cursor: pointer; }
</style>

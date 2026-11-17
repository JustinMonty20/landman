<script lang="ts">
  import { onMount } from 'svelte';
  import { Map, NavigationControl, setWorkerUrl, type GeoJSONSource } from 'maplibre-gl';
  import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url';
  import 'maplibre-gl/dist/maplibre-gl.css';
  import { geometry, parcels, type Parcel } from './discovery';

  let { records, selected, onselect, onview }: {
    records: Parcel[];
    selected: Parcel | undefined;
    onselect: (id: string) => void;
    onview: (ids: string[]) => void;
  } = $props();
  let container: HTMLDivElement;
  let map: Map | undefined;
  let ready = $state(false);
  let failed = $state(false);
  let lastLocated: string | undefined;

  function reportView() {
    if (map) onview(parcels.filter((p) => map!.getBounds().contains(p.center)).map((p) => p.id));
  }

  onMount(() => {
    try {
      setWorkerUrl(workerUrl);
      const palette = getComputedStyle(container);
      map = new Map({
        container,
        center: [-80.57, 34.95], zoom: 11.5,
        attributionControl: false,
        style: { version: 8, sources: {}, layers: [{ id: 'background', type: 'background', paint: { 'background-color': palette.getPropertyValue('--color-surface-950').trim() } }] }
      });
      map.addControl(new NavigationControl({ showCompass: false }), 'top-right');
      map.on('error', () => { failed = true; });
      map.on('load', () => {
        if (!map) return;
        map.addSource('parcel-context', { type: 'geojson', data: geometry(parcels) });
        map.addLayer({ id: 'parcel-context-fill', type: 'fill', source: 'parcel-context', paint: { 'fill-color': palette.getPropertyValue('--color-surface-900').trim(), 'fill-opacity': 0.7 } });
        map.addLayer({ id: 'parcel-context-outline', type: 'line', source: 'parcel-context', paint: { 'line-color': palette.getPropertyValue('--color-surface-500').trim(), 'line-width': 1 } });
        map.addSource('selected-parcel', { type: 'geojson', data: geometry(selected ? [selected] : []) });
        map.addSource('parcels', { type: 'geojson', data: geometry(records) });
        map.addLayer({ id: 'parcel-fill', type: 'fill', source: 'parcels', paint: { 'fill-color': '#60a5fa', 'fill-opacity': 0.55 } });
        map.addLayer({ id: 'parcel-outline', type: 'line', source: 'parcels', paint: { 'line-color': '#93c5fd', 'line-width': 1.5 } });
        map.addLayer({ id: 'selected', type: 'line', source: 'selected-parcel', paint: { 'line-color': '#edba71', 'line-width': 4 } });
        map.on('click', 'parcel-context-fill', (event) => {
          const id = event.features?.[0]?.properties?.id;
          if (typeof id === 'string') onselect(id);
        });
        map.on('mouseenter', 'parcel-context-fill', () => { if (map) map.getCanvas().style.cursor = 'pointer'; });
        map.on('mouseleave', 'parcel-context-fill', () => { if (map) map.getCanvas().style.cursor = ''; });
        ready = true;
        reportView();
      });
      map.on('moveend', reportView);
      const observer = new ResizeObserver(() => map?.resize());
      observer.observe(container);
      return () => { observer.disconnect(); map?.remove(); };
    } catch {
      failed = true;
    }
  });

  $effect(() => {
    if (!ready || !map) return;
    (map.getSource('parcels') as GeoJSONSource).setData(geometry(records));
    (map.getSource('selected-parcel') as GeoJSONSource).setData(geometry(selected ? [selected] : []));
    if (selected && selected.id !== lastLocated) {
      lastLocated = selected.id;
      map.easeTo({ center: selected.center, duration: 0 });
    }
    if (!selected) lastLocated = undefined;
  });
</script>

<div class="map-frame">
  <div class="canvas" bind:this={container} aria-label="Sample parcel map"></div>
  <div class="legend"><span class="context"></span><span class="legend-label">Parcel boundary</span> <span class="match"></span><span class="legend-label">Matching parcels ({records.length})</span> <span class="selection"></span>Selected parcel</div>
  {#if failed}<p class="failure" role="status">Map unavailable. Use filters or parcel search to inspect parcels.</p>{/if}
</div>

<style>
  .map-frame { position: relative; height: 100%; min-height: 380px; }
  .canvas { position: absolute; inset: 0; }
  .legend, .failure { position: absolute; background: var(--surface); padding: 12px 14px; border: 1px solid var(--border); border-radius: 6px; font-size: 12px; pointer-events: none; }
  .legend { bottom: 18px; left: 18px; display: flex; gap: 8px; align-items: center; }
  .legend .context, .legend .match, .legend .selection { width: 13px; height: 13px; display: inline-block; }
  .context { background: var(--surface); border: 1px solid var(--color-surface-500); }
  .match { background: #3e648f; border: 1px solid #93c5fd; }
  .selection { border: 3px solid #edba71; }
  .failure { bottom: 80px; left: 18px; right: 18px; }
</style>

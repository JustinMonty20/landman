import type { LayerSpecification, Map } from 'maplibre-gl';

const host = 'https://tiles.openfreemap.org';
export const basemapSource = 'openfreemap';
const source = basemapSource;
export function isBasemapRequest(url: string): boolean {
  return url.startsWith(`${host}/`);
}

const labelPaint = { 'text-color': '#c4c8c4', 'text-halo-color': '#24272b', 'text-halo-width': 1.2 };
const layers: LayerSpecification[] = [
  { id: 'context-vegetation', type: 'fill', source, 'source-layer': 'landcover', minzoom: 9,
    filter: ['in', ['get', 'class'], ['literal', ['wood', 'grass']]], paint: { 'fill-color': '#35483d' } },
  { id: 'context-parks', type: 'fill', source, 'source-layer': 'park', minzoom: 9,
    paint: { 'fill-color': '#405747', 'fill-opacity': 0.75 } },
  { id: 'context-water', type: 'fill', source, 'source-layer': 'water', minzoom: 9,
    paint: { 'fill-color': '#344b56' } },
  { id: 'context-streams', type: 'line', source, 'source-layer': 'waterway', minzoom: 12,
    paint: { 'line-color': '#486573', 'line-width': 1 } },
  { id: 'context-roads', type: 'line', source, 'source-layer': 'transportation', minzoom: 9,
    filter: ['in', ['get', 'class'], ['literal', ['motorway', 'trunk', 'primary', 'secondary', 'tertiary', 'minor', 'service']]],
    paint: { 'line-color': '#777a79', 'line-width': ['interpolate', ['linear'], ['zoom'], 9,
      ['match', ['get', 'class'], ['motorway', 'trunk', 'primary'], 1, 0], 14, 1, 17, 2] } },
  { id: 'context-street-names', type: 'symbol', source, 'source-layer': 'transportation_name', minzoom: 14,
    filter: ['has', 'name'], layout: { 'symbol-placement': 'line', 'text-field': ['get', 'name'], 'text-font': ['Noto Sans Regular'], 'text-size': 11 }, paint: labelPaint },
  { id: 'context-place-names', type: 'symbol', source, 'source-layer': 'place', minzoom: 9,
    filter: ['all', ['has', 'name'], ['in', ['get', 'class'], ['literal', ['city', 'town', 'village', 'hamlet', 'suburb', 'quarter', 'neighbourhood']]]],
    layout: { 'text-field': ['get', 'name'], 'text-font': ['Noto Sans Regular'], 'text-size': 13 }, paint: labelPaint },
];

export function addBasemap(map: Map, before: string): void {
  map.setGlyphs(`${host}/fonts/{fontstack}/{range}.pbf`);
  map.addSource(source, { type: 'vector', url: `${host}/planet` });
  for (const layer of layers) map.addLayer(layer, before);
}

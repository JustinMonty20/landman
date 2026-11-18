import type { FeatureCollection, Polygon } from 'geojson';
import boundaries from './data/counties.json';
import additionalBoundaries from './data/chesterfield-stanly.json';

export const counties = [
  { id: '37179', name: 'Union', state: 'NC', status: 'configured', center: [-80.53, 34.99] },
  { id: '37007', name: 'Anson', state: 'NC', status: 'locked', center: [-80.10, 34.97] },
  { id: '45057', name: 'Lancaster', state: 'SC', status: 'locked', center: [-80.74, 34.68] },
  { id: '37119', name: 'Mecklenburg', state: 'NC', status: 'locked', center: [-80.83, 35.25] },
  { id: '37025', name: 'Cabarrus', state: 'NC', status: 'locked', center: [-80.55, 35.39] },
  { id: '45091', name: 'York', state: 'SC', status: 'locked', center: [-81.18, 34.97] },
  { id: '37071', name: 'Gaston', state: 'NC', status: 'locked', center: [-81.18, 35.30] },
  { id: '45025', name: 'Chesterfield', state: 'SC', status: 'locked', center: [-80.16, 34.64] },
  { id: '37167', name: 'Stanly', state: 'NC', status: 'locked', center: [-80.25, 35.31] }
] as const;

export type County = (typeof counties)[number];
export type CountyId = County['id'];
export type MapDetail = 'region' | 'county' | 'parcels';

export function mapDetail(zoom: number): MapDetail {
  if (zoom < 9) return 'region';
  return zoom < 13 ? 'county' : 'parcels';
}
export const countyStatus = (county: County) => county.status === 'configured'
  ? 'Ingestion configured' : 'Locked — ingestion not configured';

export const countyGeometry: FeatureCollection<Polygon> = {
  type: 'FeatureCollection',
  features: [...boundaries.features, ...additionalBoundaries.features].map((feature) => {
    const county = counties.find((county) => county.id === feature.properties.GEOID);
    if (!county) throw new Error('Unknown county boundary');
    return {
      type: 'Feature', id: county.id,
      properties: { id: county.id, name: county.name, status: county.status },
      geometry: { type: 'Polygon', coordinates: feature.geometry.coordinates }
    };
  })
};

export function countyBounds(id?: CountyId): [[number, number], [number, number]] {
  const positions = countyGeometry.features.filter((feature) => !id || feature.id === id)
    .flatMap((feature) => feature.geometry.coordinates.flat());
  return [
    [Math.min(...positions.map(([x]) => x)), Math.min(...positions.map(([, y]) => y))],
    [Math.max(...positions.map(([x]) => x)), Math.max(...positions.map(([, y]) => y))]
  ];
}

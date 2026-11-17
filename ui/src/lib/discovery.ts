import type { FeatureCollection, Polygon } from 'geojson';

export type Evidence = 'yes' | 'no' | 'unknown';
export type Parcel = {
  id: string;
  address: string;
  acres: number;
  vacancy: Evidence;
  mailing: 'NC' | 'SC' | 'VA' | null;
  llc: Evidence;
  owner: string;
  center: [number, number];
  stale: boolean;
};
export type Filters = { min: string; max: string; vacant: boolean; outside: boolean; llc: boolean };
export const emptyFilters = (): Filters => ({ min: '', max: '', vacant: false, outside: false, llc: false });

// These fictional records support layout review, not county evidence or performance checks.
export const parcels: Parcel[] = Array.from({ length: 24 }, (_, i) => ({
  id: `DEMO-${String(i + 1).padStart(3, '0')}`,
  address: `${100 + i * 12} ${['Sample Oak Lane', 'Sample Meadow Road', 'Sample Creek Drive', 'Sample Pine Road'][i % 4]}`,
  acres: [1.2, 3.8, 8.4, 12.6, 24.1, 6.5][i % 6],
  vacancy: (['yes', 'yes', 'unknown', 'no'] as const)[i % 4],
  mailing: (['SC', 'NC', null, 'VA', 'NC', 'SC'] as const)[i % 6],
  llc: (['yes', 'no', 'unknown'] as const)[i % 3],
  owner: i % 3 === 0 ? `Sample Land ${i + 1} LLC` : `Sample Owner ${i + 1}`,
  center: [-80.64 + (i % 6) * 0.028, 34.91 + Math.floor(i / 6) * 0.026],
  stale: i % 7 === 0
}));

export function acreageError(filters: Filters): string {
  const values = [filters.min, filters.max];
  if (values.some((v) => v !== '' && (!Number.isFinite(Number(v)) || Number(v) < 0))) return 'Enter an acreage of zero or more.';
  if (filters.min !== '' && filters.max !== '' && Number(filters.min) > Number(filters.max)) return 'Minimum acreage must not exceed maximum acreage.';
  return '';
}

export function parcelSearchLabel(parcel: Parcel): string {
  return `${parcel.address} · ${parcel.id}`;
}

export function searchParcels(records: Parcel[], value: string): Parcel[] {
  const query = value.trim().toLowerCase();
  if (!query) return [];
  return records.filter((parcel) => parcel.id.toLowerCase().includes(query) || parcel.address.toLowerCase().includes(query));
}

export function hasActiveFilters(filters: Filters): boolean {
  return filters.min !== '' || filters.max !== '' || filters.vacant || filters.outside || filters.llc;
}

export function matches(parcel: Parcel, filters: Filters): boolean {
  if (!hasActiveFilters(filters) || acreageError(filters)) return false;
  return (filters.min === '' || parcel.acres >= Number(filters.min))
    && (filters.max === '' || parcel.acres <= Number(filters.max))
    && (!filters.vacant || parcel.vacancy === 'yes')
    && (!filters.outside || (parcel.mailing !== null && parcel.mailing !== 'NC'))
    && (!filters.llc || parcel.llc === 'yes');
}

export function geometry(records: Parcel[]): FeatureCollection<Polygon> {
  return { type: 'FeatureCollection', features: records.map((p) => {
    const [x, y] = p.center;
    return { type: 'Feature', id: p.id, properties: { id: p.id }, geometry: { type: 'Polygon', coordinates: [[[x - .005, y - .004], [x + .005, y - .004], [x + .005, y + .004], [x - .005, y + .004], [x - .005, y - .004]]] } };
  }) };
}

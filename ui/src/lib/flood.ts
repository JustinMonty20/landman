import type { FeatureCollection, Geometry } from 'geojson';

export type Viewport = { bbox: string; zoom: number };

export function viewportQuery(view: Viewport): URLSearchParams {
  return new URLSearchParams({ bbox: view.bbox, zoom: String(view.zoom) });
}
export type ParcelShape = { id: string; parcel_number: string | null; address: string | null; geometry: Geometry };
export type FloodRelease = { id: string; issued_at: string; observed_at: string };
export type FloodMap = FeatureCollection & { release: FloodRelease | null };
export type FloodDetail = { parcel_number: string; release: FloodRelease | null } & (
  | { status: 'no_release' | 'missing_geometry' }
  | { status: 'available'; acres: { parcel: number; unknown: number; floodway: number };
      zones: { code: string; subtype: string; sfha: string; acres: number }[];
      panels: { number: string; effective_date: string | null }[] }
);

export async function readMapData<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal, credentials: 'same-origin', cache: 'no-store' });
  if (!response.ok) throw new Error(String(response.status));
  return response.json() as Promise<T>;
}

export function parcelShapes(items: ParcelShape[]): FeatureCollection {
  return { type: 'FeatureCollection', features: items.map(({ id, geometry }) => ({ type: 'Feature', geometry, properties: { id } })) };
}

export const emptyShapes: FeatureCollection = { type: 'FeatureCollection', features: [] };

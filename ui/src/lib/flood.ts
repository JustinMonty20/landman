import type { FeatureCollection, Geometry } from 'geojson';

export const floodStyles = {
  floodway: { color: '#dc78bf', label: 'Regulatory floodway' },
  sfha: { color: '#65cfe4', label: 'Special flood hazard area (1% annual chance)' },
  moderate: { color: '#f4aa59', label: 'Zone X — 0.2% annual chance' },
  minimal: { color: '#8b9ee8', label: 'Zone X — minimal flood hazard' },
  unclassified: { color: '#afb3bc', label: 'Unclassified mapped zone' }
};

export type Viewport = { bbox: string; zoom: number };

export function viewportQuery(view: Viewport): URLSearchParams {
  return new URLSearchParams({ bbox: view.bbox, zoom: String(view.zoom) });
}
export type ParcelShape = { id: string; parcel_number: string | null; address: string | null; geometry: Geometry };
export type FloodRelease = { id: string; issued_at: string; observed_at: string };
export type FloodMap = FeatureCollection & { release: FloodRelease | null; overview: boolean };
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

export function mapLayerQuery(view: Viewport): string {
  return view.zoom < 13 ? 'overview=true' : viewportQuery(view).toString();
}

export async function readMapLayer<T>(path: string, signal: AbortSignal): Promise<T> {
  try {
    return await readMapData<T>(path, signal);
  } catch (error) {
    const [route, query] = path.split('?');
    const params = new URLSearchParams(query);
    if (!(error instanceof Error) || error.message !== '413' || params.has('overview')) throw error;
    params.delete('bbox');
    params.delete('zoom');
    params.set('overview', 'true');
    return readMapData<T>(`${route}?${params}`, signal);
  }
}

export function parcelShapes(items: ParcelShape[]): FeatureCollection {
  return { type: 'FeatureCollection', features: items.map(({ id, geometry }) => ({ type: 'Feature', geometry, properties: { id } })) };
}

export const emptyShapes: FeatureCollection = { type: 'FeatureCollection', features: [] };

export const detailGroups = [
  { id: 'acreage', label: 'Acreage' },
  { id: 'ownership', label: 'Ownership and sales' },
  { id: 'tax', label: 'Tax' },
  { id: 'vacancy', label: 'Vacancy' },
  { id: 'soil', label: 'Soil' },
  { id: 'zoning', label: 'Zoning' },
  { id: 'roads', label: 'Roads' },
  { id: 'flood', label: 'Flood' }
] as const;
export type DetailGroup = typeof detailGroups[number]['id'];
export const defaultDetailGroups: DetailGroup[] = ['acreage', 'soil'];
export function parseDetailGroups(value: unknown): DetailGroup[] {
  if (!value || typeof value !== 'object' || !('groups' in value) || !Array.isArray(value.groups) ||
    value.groups.some((group: unknown) => !detailGroups.some(item => item.id === group)) ||
    new Set(value.groups).size !== value.groups.length) throw new Error('Invalid preference response');
  const groups = value.groups as DetailGroup[];
  return detailGroups.filter(item => groups.includes(item.id)).map(item => item.id);
}

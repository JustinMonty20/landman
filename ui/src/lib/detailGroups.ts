// Group labels organize the picker only. IDs are individual display fields, not database projections.
export const detailGroups = [
 { label: 'Acreage', fields: [{ id: 'mapped_acres', label: 'Mapped acreage' }] },
 { label: 'Ownership', fields: [{ id: 'owner_name', label: 'Owner name' }, { id: 'entity_indicator', label: 'Entity indicator' }, { id: 'sale_count', label: 'Stored sale count' }] },
 { label: 'Tax', fields: [{ id: 'delinquent_total', label: 'Reported delinquent total' }, { id: 'tax_bill_count', label: 'Stored tax bill count' }, { id: 'foreclosure_count', label: 'Foreclosure case count' }] },
 { label: 'Vacancy', fields: [{ id: 'vacancy_tier', label: 'Vacancy tier' }, { id: 'vacancy_reason', label: 'Vacancy reason' }] },
 { label: 'Soil', fields: [{ id: 'soil_acres', label: 'Assessed soil acreage' }, { id: 'soil_septic', label: 'Septic field rating' }, { id: 'soil_dwelling_without', label: 'Dwelling without basement rating' }, { id: 'soil_dwelling_with', label: 'Dwelling with basement rating' }] },
 { label: 'Zoning', fields: [{ id: 'zoning_district', label: 'Zoning district' }, { id: 'zoning_jurisdiction', label: 'Jurisdiction' }, { id: 'zoning_unmapped', label: 'Unmapped acreage' }] },
 { label: 'Roads', fields: [{ id: 'road_distance', label: 'Mapped road distance' }, { id: 'road_name', label: 'Nearest road name' }, { id: 'road_status', label: 'Road status' }] },
 { label: 'Flood', fields: [{ id: 'flood_unknown', label: 'Unknown coverage acreage' }, { id: 'floodway_acres', label: 'Mapped floodway acreage' }, { id: 'flood_zones', label: 'Mapped flood zones' }] }
] as const;
export type DetailField = typeof detailGroups[number]['fields'][number]['id'];
export const detailFields = detailGroups.flatMap(group => [...group.fields]) as { id: DetailField; label: string }[];
export const defaultDetailFields: DetailField[] = ['mapped_acres', 'soil_septic'];
export function parseDetailFields(value: unknown): DetailField[] {
 if (!value || typeof value !== 'object' || !('fields' in value) || !Array.isArray(value.fields) ||
  value.fields.some((field: unknown) => !detailFields.some(item => item.id === field)) ||
  new Set(value.fields).size !== value.fields.length) throw new Error('Invalid preference response');
 const fields = value.fields as DetailField[];
 return detailFields.filter(item => fields.includes(item.id)).map(item => item.id);
}

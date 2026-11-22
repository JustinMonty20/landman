export type FilterDraft = {
  businessName: boolean;
  outOfState: boolean;
  delinquent: boolean;
  foreclosure: boolean;
  minAcres: number | undefined;
  maxAcres: number | undefined;
  likelyVacant: boolean;
  soilType: string;
  soilUse: string;
  soilRating: string;
};
export type FilterGroup = 'Owner' | 'Tax' | 'Parcel';
export type FilterSelection = { key: keyof FilterDraft; group: FilterGroup; label: string };

export function emptyFilterDraft(): FilterDraft {
  return {
    businessName: false, outOfState: false, delinquent: false, foreclosure: false,
    minAcres: undefined, maxAcres: undefined, likelyVacant: false, soilType: '', soilUse: '', soilRating: ''
  };
}

export function filterSelections(filters: FilterDraft): FilterSelection[] {
  const selections: FilterSelection[] = [];
  if (filters.businessName) selections.push({ key: 'businessName', group: 'Owner', label: 'Entity-name signal' });
  if (filters.outOfState) selections.push({ key: 'outOfState', group: 'Owner', label: 'Out-of-state mailing' });
  if (filters.delinquent) selections.push({ key: 'delinquent', group: 'Tax', label: 'Reported delinquent taxes' });
  if (filters.foreclosure) selections.push({ key: 'foreclosure', group: 'Tax', label: 'Foreclosure record' });
  if (filters.minAcres !== undefined) selections.push({ key: 'minAcres', group: 'Parcel', label: `At least ${filters.minAcres} acres` });
  if (filters.maxAcres !== undefined) selections.push({ key: 'maxAcres', group: 'Parcel', label: `At most ${filters.maxAcres} acres` });
  if (filters.soilType) selections.push({ key: 'soilType', group: 'Parcel', label: `Soil unit ${filters.soilType}` });
  if (filters.soilRating) selections.push({ key: 'soilRating', group: 'Parcel', label: `${filters.soilUse}: ${filters.soilRating}` });
  if (filters.likelyVacant) selections.push({ key: 'likelyVacant', group: 'Parcel', label: 'Likely vacant' });
  return selections;
}

export function removeDraftFilter(filters: FilterDraft, key: keyof FilterDraft): FilterDraft {
  if (key === 'minAcres' || key === 'maxAcres') return { ...filters, [key]: undefined };
  if (key === 'soilRating') return { ...filters, soilRating: '', soilUse: '' };
  if (key === 'soilType' || key === 'soilUse') return { ...filters, [key]: '' };
  return { ...filters, [key]: false };
}

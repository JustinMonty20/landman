// Shared across the parcel detail and Notes route during client-side navigation.
export const noteDrafts = $state<Record<string,string>>({});
export const noteEdits = $state<Record<string,string>>({});
export const noteRetries = $state<Record<string,{id:string;body:string}>>({});

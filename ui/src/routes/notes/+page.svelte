<script lang="ts">
import { page } from '$app/state';
import ParcelNotes from '../../lib/ParcelNotes.svelte';
import { noteDrafts, noteEdits, noteRetries } from '../../lib/notesState.svelte';
const parcel = $derived(page.url.searchParams.get('parcel') ?? '');
const valid = $derived(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(parcel));
</script>
<svelte:head><title>Landman · Parcel notes</title></svelte:head>
<main class="timeline-page">
 <a class="return-link" href={valid?`/?parcel=${encodeURIComponent(parcel)}`:'/'}>← Return to parcel</a>
 {#if valid}
 <p class="parcel-identity">Parcel {parcel}</p>
 {#key parcel}<ParcelNotes {parcel} mode="timeline" drafts={noteDrafts} edits={noteEdits} retries={noteRetries}/>{/key}
 {:else}<h1>Parcel notes unavailable</h1><p>Open a parcel to view its notes.</p>{/if}
</main>
<style>
.timeline-page{max-width:840px;margin:0 auto;padding:24px 20px 72px}.return-link{display:inline-block;margin-bottom:18px;color:var(--color-primary-300)}.parcel-identity{font-size:12px;color:var(--muted);overflow-wrap:anywhere}.timeline-page :global(.notes){padding:16px 0}
</style>

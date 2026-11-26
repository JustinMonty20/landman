<script lang="ts">
import { page } from '$app/state';
import { counties, countyStatus } from '../../lib/counties';
const selected = $derived(counties.find((county) => county.id === page.url.searchParams.get('county')));
</script>
<svelte:head>
  <title>Landman · County coverage</title>
  <meta name="description" content="County ingestion configuration and parcel display availability." />
</svelte:head>
<div class="app">
  <main class="coverage-page" aria-label="County coverage">
    <div class="content">
      <a class="back" href={selected ? `/?county=${selected.id}` : '/'}>← Return to map</a>
      <h1>County coverage</h1>
      <p>{counties.filter((county) => county.status === 'configured').length} county configured · {counties.filter((county) => county.status === 'locked').length} counties locked</p>
      <p>Configured means that an ingestion job exists. It does not confirm a successful import or current source data.</p>
      <p>Union parcel display is not connected yet. No parcels are shown in the application.</p>
      <ul aria-label="County ingestion status">
        {#each counties as county (county.id)}
          <li>
            <div><h2>{county.name} County, {county.state}</h2><p class="status">{countyStatus(county)}</p>
              <p>{county.status === 'configured' ? 'Ingestion is configured. Parcel display is not connected yet.' : 'Parcel discovery is not available. The map shows the county outline only.'}</p>
            </div>
            <a class="btn btn-sm preset-outlined-surface-300-700 bg-surface-900" href={`/?county=${county.id}`} aria-label={`Show ${county.name} County on map`}>Show on map</a>
          </li>
        {/each}
      </ul>
      <section aria-labelledby="legend-heading">
        <h2 id="legend-heading">Map legend</h2>
        <p>Blue fill identifies Union County. Gray fill identifies locked counties. An amber outline identifies the selected county. County labels also identify locked counties.</p>
        <p>County outlines come from the U.S. Census Bureau. They are simplified for display, not legal boundaries or parcel evidence.</p>
      </section>
    </div>
  </main>
</div>
<style>
.coverage-page { overflow: auto; }
.content { max-width: 900px; margin: 0 auto; padding: 24px; }
.back { display: inline-block; color: #93c5fd; margin-bottom: 20px; }
h1 { font-size: 26px; font-weight: 650; }
h2 { font-size: 17px; font-weight: 600; }
p { margin: 10px 0; font-size: 13px; line-height: 1.6; color: var(--muted); }
ul { display: grid; gap: 12px; margin: 24px 0; padding: 0; list-style: none; }
li { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 18px; border: 1px solid var(--border); border-radius: 6px; background: var(--surface); }
li p:last-child { margin-bottom: 0; }
li a { flex-shrink: 0; }
.status { color: #dceaff; }
section { padding: 20px 0; border-top: 1px solid var(--border); }
@media (max-width: 600px) { li { align-items: start; flex-direction: column; } .content { padding: 16px; } }
</style>

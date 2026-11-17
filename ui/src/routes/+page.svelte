<script lang="ts">
  import { tick } from 'svelte';
  import { Pagination } from '@skeletonlabs/skeleton-svelte';
  import ParcelMap from '../lib/ParcelMap.svelte';
  import { acreageError, emptyFilters, hasActiveFilters, matches, parcelSearchLabel, parcels, searchParcels, type Filters, type Parcel } from '../lib/discovery';
  import '../style.css';

  let panelOpen = $state(false);
  let controlsHeight = $state(0);
  let matchesVisible = $state(true);
  let detailHeading = $state<HTMLHeadingElement>();
  let filters = $state(emptyFilters());
  let selection = $state<{ id: string; origin: 'match' | 'lookup' }>();
  let viewIds = $state<string[]>([]);
  let mode = $state('all');
  let page = $state(0);
  let minValue = $state<number | undefined>();
  let maxValue = $state<number | undefined>();
  let search = $state('');
  let searchTarget = $state<Parcel>();
  function reset() { filters = emptyFilters(); minValue = undefined; maxValue = undefined; }
  const pageSize = 6;
  const searchMatches = $derived(searchParcels(parcels, search));
  const matched = $derived(parcels.filter((p) => matches(p, filters)));
  const inView = $derived(matched.filter((p) => viewIds.includes(p.id)));
  const displayed = $derived(mode === 'all' ? matched : inView);
  const pageCount = $derived(Math.max(1, Math.ceil(displayed.length / pageSize)));
  const selected = $derived(parcels.find((p) => p.id === selection?.id));
  const filtering = $derived(hasActiveFilters(filters));
  const error = $derived(acreageError(filters));
  const activeFilters = $derived([
    ...(filters.min !== '' ? [{ key: 'min' as const, label: `At least ${filters.min} acres` }] : []),
    ...(filters.max !== '' ? [{ key: 'max' as const, label: `At most ${filters.max} acres` }] : []),
    ...(filters.vacant ? [{ key: 'vacant' as const, label: 'Likely vacant' }] : []),
    ...(filters.outside ? [{ key: 'outside' as const, label: 'Mailing outside NC' }] : []),
    ...(filters.llc ? [{ key: 'llc' as const, label: 'LLC-name match' }] : [])
  ]);

  async function removeFilter(key: keyof Filters) {
    const index = activeFilters.findIndex((filter) => filter.key === key);
    if (key === 'min') { filters.min = ''; minValue = undefined; }
    else if (key === 'max') { filters.max = ''; maxValue = undefined; }
    else filters[key] = false;
    await tick();
    const next = activeFilters[Math.min(index, activeFilters.length - 1)];
    (next ? document.getElementById(`active-filter-${next.key}`) : document.getElementById('filter-toggle'))?.focus({ preventScroll: true });
  }

  $effect(() => { filters.min; filters.max; filters.vacant; filters.outside; filters.llc; page = 0; });
  $effect(() => { if (page >= pageCount) page = pageCount - 1; });
  $effect(() => { detailHeading?.focus({ preventScroll: true }); });
  $effect(() => { if (selection?.origin === 'match' && !matched.some((p) => p.id === selection?.id)) selection = undefined; });

  async function closeDetails() {
    const id = selection?.id;
    selection = undefined;
    await tick();
    (document.getElementById(`result-${id}`) ?? document.getElementById('parcel-finder'))?.focus({ preventScroll: true });
  }

  function chooseSearch(parcel: Parcel) {
    search = parcelSearchLabel(parcel);
    searchTarget = parcel;
    select(parcel.id, 'lookup');
  }
  function submitSearch() {
    if (searchMatches.length === 1) chooseSearch(searchMatches[0]);
  }
  function select(id: string, origin: 'match' | 'lookup' = 'match') {
    if (!(origin === 'lookup' ? parcels : matched).some((parcel) => parcel.id === id)) return;
    selection = { id, origin };
    matchesVisible = true;
    if (origin === 'lookup') return;
    let index = displayed.findIndex((p) => p.id === id);
    if (index < 0) {
      mode = 'all';
      index = matched.findIndex((p) => p.id === id);
    }
    page = Math.floor(index / pageSize);
  }
</script>

<svelte:head><title>Landman · Discovery prototype</title><meta name="description" content="Review parcel discovery with fictional sample data." /></svelte:head>

{#snippet filterControls()}
  <div class="acreage">
    <label>Minimum acres<input class="input" type="number" min="0" step="any" placeholder="Any" bind:value={minValue} oninput={(e) => filters.min = e.currentTarget.value} /></label>
    <label>Maximum acres<input class="input" type="number" min="0" step="any" placeholder="Any" bind:value={maxValue} oninput={(e) => filters.max = e.currentTarget.value} /></label>
  </div>
  <label class="check"><input class="checkbox" type="checkbox" bind:checked={filters.vacant} />Likely vacant</label>
  <label class="check"><input class="checkbox" type="checkbox" bind:checked={filters.outside} />Mailing outside NC</label>
  <label class="check"><input class="checkbox" type="checkbox" bind:checked={filters.llc} />LLC-name match</label>
  <button class="btn btn-sm text-button" onclick={reset}>Clear filters</button>
{/snippet}

<div class="app">
  <header>
    <a class="brand" href="/">▧ <span>landman</span></a>
  </header>
  <main aria-label="Parcel discovery">
    <section class="map-area" aria-label="Parcel map">
      <ParcelMap records={matched} {selected} onselect={(id) => select(id, matched.some((parcel) => parcel.id === id) ? 'match' : 'lookup')} onview={(ids) => viewIds = ids} />
    </section>
    <div class="map-controls" bind:clientHeight={controlsHeight}>
    <form class="search-row" onsubmit={(event) => { event.preventDefault(); submitSearch(); }}>
      <label class="search"><span>Find parcel ID or address</span><input id="parcel-finder" class="input" type="search" role="combobox" aria-autocomplete="list" placeholder="Try DEMO-001 or Sample Oak Lane" bind:value={search} oninput={() => searchTarget = undefined} aria-controls="parcel-suggestions" aria-expanded={searchMatches.length > 0 && !searchTarget} /></label>
      {#if searchMatches.length && !searchTarget}<div class="search-suggestions" id="parcel-suggestions" role="listbox" aria-label="Parcel suggestions">
        {#each searchMatches.slice(0, 8) as parcel}<button type="button" role="option" aria-selected="false" onclick={() => chooseSearch(parcel)}>{parcelSearchLabel(parcel)}</button>{/each}
      </div>{/if}
    </form>
    <div class="filter-summary" aria-live="polite">
      {#if activeFilters.length}<span>Active filters:</span>{/if}
      {#each activeFilters as filter (filter.key)}<button id={`active-filter-${filter.key}`} class="btn btn-sm chip" aria-label={`Remove ${filter.label}`} title={`Remove ${filter.label}`} onclick={() => removeFilter(filter.key)}>{filter.label}</button>{/each}
      {#if error}<strong role="alert">{error}</strong>{/if}
    </div>
    </div>
    <div class="panels" style:top={`${Math.max(112, controlsHeight + 24)}px`}>
      <button id="filter-toggle" class="btn-icon preset-outlined-surface-300-700 bg-surface-900 edge-toggle left-toggle" class:open={panelOpen} aria-label={panelOpen ? 'Hide filters' : 'Show filters'} aria-expanded={panelOpen} aria-controls="filters" onclick={() => panelOpen = !panelOpen}>{panelOpen ? '‹' : '›'}</button>
      <button id="matches-toggle" class="btn-icon preset-outlined-surface-300-700 bg-surface-900 edge-toggle right-toggle" class:open={matchesVisible} aria-label={selected ? (matchesVisible ? 'Hide parcel details panel' : 'Show parcel details panel') : (matchesVisible ? 'Hide matches' : 'Show matches')} aria-expanded={matchesVisible} aria-controls="parcel-panel" onclick={() => matchesVisible = !matchesVisible}>{matchesVisible ? '›' : '‹'}</button>
      <div class="left-panels" class:collapsed={!panelOpen} inert={!panelOpen} aria-hidden={!panelOpen}>
        <aside class="filter-panel" id="filters" aria-label="Parcel filters"><h2>Filter parcels</h2><p>Select any combination.</p>{@render filterControls()}<p class="fine">Mailing state does not prove owner residence. LLC is a name match, not verified registration. Unknown values do not pass positive filters.</p></aside>
      </div>
      <div id="parcel-panel" class="right-panel" class:collapsed={!matchesVisible} inert={!matchesVisible} aria-hidden={!matchesVisible}>
      {#if !selected}<section class="results" id="results" aria-label="Matching parcels">
        <div class="results-heading"><h2>{matched.length} matches</h2><p>{inView.length} in map view · {matched.length} in sample county set</p>
          <label class="list-mode">Show<select class="select field-sm" bind:value={mode} onchange={() => page = 0}><option value="all">All matches</option><option value="view">Matches in map view</option></select></label>
        </div>
        <div class="result-rows">
          {#each displayed.slice(page * pageSize, (page + 1) * pageSize) as parcel}
            <button class="parcel-row" id={`result-${parcel.id}`} onclick={() => select(parcel.id)}>
              <span class="row-top"><strong>{parcel.acres.toFixed(1)} acres</strong><span>{parcel.id}</span></span>
              <span class="address">{parcel.address}</span>
              <span class="row-facts">{parcel.vacancy === 'yes' ? 'Likely vacant' : parcel.vacancy === 'unknown' ? 'Vacancy unknown' : 'Vacancy signal: no'} · {parcel.mailing === null ? 'Mailing unknown' : parcel.mailing === 'NC' ? 'NC mailing' : 'Mailing outside NC'}</span>
              <span class="row-facts">{parcel.llc === 'yes' ? 'LLC-name match' : parcel.llc === 'unknown' ? 'LLC status unknown' : 'No LLC-name match'}{parcel.stale ? ' · Stale sample evidence' : ''}</span>
            </button>
          {:else}<div class="empty">{#if !filtering}<h3>Choose filters to find parcels</h3><p>Apply at least one filter. Use search to locate a specific parcel.</p>{:else}<h3>No matching parcels</h3><p>Change the filters.{mode === 'view' ? ' You can also move the map or show all matches.' : ''}</p>{/if}</div>{/each}
        </div>
        <Pagination class="pagination" role="navigation" aria-label="Result pages" count={displayed.length} pageSize={pageSize} page={page + 1} onPageChange={(event) => page = event.page - 1}>
          <Pagination.PrevTrigger aria-label="Previous">Previous</Pagination.PrevTrigger>
          <span>Page {page + 1} of {pageCount}</span>
          <Pagination.NextTrigger aria-label="Next">Next</Pagination.NextTrigger>
        </Pagination>
      </section>{:else}
          <section class="details" aria-label="Parcel details">
            <div class="detail-heading"><div><p class="eyebrow">SELECTED SAMPLE PARCEL</p><h2 tabindex="-1" bind:this={detailHeading}>{selected.address}</h2></div><button class="btn-icon preset-outlined-surface-300-700 bg-surface-900" aria-label="Close parcel details" onclick={closeDetails}>×</button></div>
            {#if selection?.origin === 'lookup' && !matches(selected, filters)}<p class="fine">This parcel is not in the current filter results.</p>{/if}
            <dl><div><dt>Parcel ID</dt><dd>{selected.id}</dd></div><div><dt>Area</dt><dd>{selected.acres} acres</dd></div><div><dt>Owner</dt><dd>{selected.owner}</dd></div><div><dt>Mailing state</dt><dd>{selected.mailing ?? 'Unknown'}</dd></div></dl>
            <p>Vacancy: {selected.vacancy === 'yes' ? 'provisional likely-vacant signal' : selected.vacancy === 'unknown' ? 'unknown' : 'no likely-vacant signal'}. Not a verified vacancy finding.</p>
            <p class="evidence-note">Fictional evidence date: 2026-01-15 · {selected.stale ? 'Stale example' : 'Sample only; freshness is not verified'}. Tax, zoning, road, and flood evidence are not connected.</p>
            <p class="fine">Saving, scratchpad entries, lead states, tax requests, and reminders are not implemented in this layout prototype.</p>
          </section>
        {/if}
      </div>
    </div>
    <div class="sample-banner">Sample data only · Fictional parcels and boundaries · Nothing is saved</div>
  </main>
</div>

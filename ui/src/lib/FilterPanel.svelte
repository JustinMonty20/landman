<script lang="ts">
import { tick } from 'svelte';
import { Accordion } from '@skeletonlabs/skeleton-svelte';
import { emptyFilterDraft, filterSelections, removeDraftFilter, type FilterDraft, type FilterGroup } from './filterDraft';

let { county, filters = $bindable(), soilChoices = [] }: { county: string; filters: FilterDraft; soilChoices?: {key: string; name: string}[] } = $props();
const selections = $derived(filterSelections(filters));
const count = (group: FilterGroup) => selections.filter((selection) => selection.group === group).length;
const acreageError = $derived(
  (filters.minAcres !== undefined && filters.minAcres < 0) || (filters.maxAcres !== undefined && filters.maxAcres < 0)
    ? 'Enter an acreage of zero or more.'
    : filters.minAcres !== undefined && filters.maxAcres !== undefined && filters.minAcres > filters.maxAcres
      ? 'Minimum acreage must not exceed maximum acreage.' : ''
);
async function remove(key: keyof FilterDraft) {
  const index = selections.findIndex((selection) => selection.key === key);
  filters = removeDraftFilter(filters, key);
  await tick();
  const next = selections[Math.min(index, selections.length - 1)];
  (document.getElementById(next ? `draft-filter-${next.key}` : 'clear-preview-filters'))?.focus({ preventScroll: true });
}
</script>

<aside class="grouped-filters" aria-label="Parcel filters">
  <div class="panel-heading">
    <h2>Filter {county} parcels</h2>
    <p>Choose filters to narrow county parcels.</p>
  </div>
  <div class="filter-scroll" role="region" aria-label="Filter sections">
    <Accordion class="filter-accordion" multiple collapsible defaultValue={['owner', 'tax']}>
      <Accordion.Item value="owner">
        <h3><Accordion.ItemTrigger class="section-trigger" aria-label="Owner filters"><span>Owner</span>{#if count('Owner')}<span class="section-count">{count('Owner')}</span>{/if}<span class="chevron" aria-hidden="true">⌄</span></Accordion.ItemTrigger></h3>
        <Accordion.ItemContent>
          <div class="section-fields">
            <label class="check"><input class="checkbox" type="checkbox" bind:checked={filters.businessName} />Entity-name signal</label>
            <p class="field-help">Business terms in the owner name, including LLCs. Not verified registration.</p>
            <label class="check"><input class="checkbox" type="checkbox" bind:checked={filters.outOfState} />Out-of-state mailing</label>
            <p class="field-help">Owner mailing address is outside North Carolina.</p>
          </div>
        </Accordion.ItemContent>
      </Accordion.Item>
      <Accordion.Item value="soil">
        <h3><Accordion.ItemTrigger class="section-trigger" aria-label="Soil filters"><span>Soil</span><span class="chevron" aria-hidden="true">⌄</span></Accordion.ItemTrigger></h3>
        <Accordion.ItemContent>
          <div class="section-fields">
            <label class="soil-type">Soil type<select class="select" bind:value={filters.soilType} aria-describedby="soil-options-note"><option value="">Any soil unit</option>{#each soilChoices as choice}<option value={choice.key}>{choice.name}</option>{/each}</select></label>
            <p class="field-help" id="soil-options-note">Choices will come from the county soil data. No text search or sample soil types.</p>
          <label class="soil-type">Intended use<select class="select" bind:value={filters.soilUse} onchange={() => filters.soilRating = ''}><option value="">Any use</option><option value="conventional-septic">Conventional septic</option><option value="dwelling-without-basement">Dwelling without basement</option><option value="dwelling-with-basement">Dwelling with basement</option></select></label>
            <label class="soil-type">{filters.soilUse ? 'Published rating (required)' : 'Published rating'}<select class="select" disabled={!filters.soilUse} required={!!filters.soilUse} aria-invalid={!!filters.soilUse && !filters.soilRating} aria-describedby="soil-rating-help" bind:value={filters.soilRating}><option value="">Select a rating</option><option value="Not limited">Not limited</option><option value="Somewhat limited">Somewhat limited</option><option value="Very limited">Very limited</option></select></label>
            <p class="field-help" id="soil-rating-help">Choose an intended use and a published soil rating to filter parcels. This does not confirm zoning permission or site suitability.</p>
            {#if filters.soilUse && !filters.soilRating}<p class="validation" role="alert">Select a published rating to apply this intended use.</p>{/if}
          </div>
        </Accordion.ItemContent>
      </Accordion.Item>
      <Accordion.Item value="tax">
        <h3><Accordion.ItemTrigger class="section-trigger" aria-label="Tax filters"><span>Tax</span>{#if count('Tax')}<span class="section-count">{count('Tax')}</span>{/if}<span class="chevron" aria-hidden="true">⌄</span></Accordion.ItemTrigger></h3>
        <Accordion.ItemContent>
          <div class="section-fields">
            <label class="check"><input class="checkbox" type="checkbox" bind:checked={filters.delinquent} />Reported delinquent taxes</label>
            <label class="check"><input class="checkbox" type="checkbox" bind:checked={filters.foreclosure} />Foreclosure record</label>
            <p class="field-help">Reported records, not a current balance or legal status. Source dates will appear with real data.</p>
          </div>
        </Accordion.ItemContent>
      </Accordion.Item>
      <Accordion.Item value="parcel">
        <h3><Accordion.ItemTrigger class="section-trigger" aria-label="Parcel filters"><span>Parcel</span>{#if count('Parcel')}<span class="section-count">{count('Parcel')}</span>{/if}<span class="chevron" aria-hidden="true">⌄</span></Accordion.ItemTrigger></h3>
        <Accordion.ItemContent>
          <div class="section-fields">
            <div class="acreage">
              <label>Minimum acres<input class="input" type="number" min="0" step="any" placeholder="Any" bind:value={filters.minAcres} /></label>
              <label>Maximum acres<input class="input" type="number" min="0" step="any" placeholder="Any" bind:value={filters.maxAcres} /></label>
            </div>
            {#if acreageError}<p class="validation" role="alert">{acreageError}</p>{/if}
            <label class="soil-type">Vacancy tier<select class="select" bind:value={filters.vacancyTier}><option value="">Any tier</option><option value="strong_candidate">Strong candidate</option><option value="review">Review</option><option value="conflict">Conflict</option><option value="unknown">Unknown</option></select></label>
            <p class="field-help">Unknown means no current, validated assessment. A candidate is not verified vacant.</p>
          </div>
        </Accordion.ItemContent>
      </Accordion.Item>
    </Accordion>
    {#if selections.length}
      <div class="preview-selections" aria-label="Selected filters">
        {#each selections as selection (selection.key)}
          <button id={`draft-filter-${selection.key}`} class="btn btn-sm chip" aria-label={`Remove ${selection.label}`} onclick={() => remove(selection.key)}>{selection.label}<span aria-hidden="true"> ×</span></button>
        {/each}
      </div>
    {/if}
  </div>
  <div class="panel-footer">
    <span aria-live="polite">{selections.length} {selections.length === 1 ? 'filter' : 'filters'} selected</span>
    <button id="clear-preview-filters" class="btn btn-sm text-button" onclick={() => filters = emptyFilterDraft()}>Clear all</button>
  </div>
</aside>

<style>
.grouped-filters { padding: 0; height: 100%; display: flex; flex-direction: column; overflow: hidden; }
.panel-heading { padding: 16px; border-bottom: 1px solid var(--border); }
h2 { font-size: 15px; }
.panel-heading p { font-size: 11px; color: var(--muted); line-height: 1.5; margin-top: 10px; }
.filter-scroll { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; }
:global(.filter-accordion [data-part='item']) { border-bottom: 1px solid var(--border); }
:global(.filter-accordion .section-trigger) { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 48px; padding: 12px 16px; font-size: 13px; font-weight: 650; text-align: left; cursor: pointer; }
:global(.filter-accordion .section-trigger:hover) { background: var(--color-surface-800); }
:global(.filter-accordion .section-trigger:focus-visible) { outline-offset: -3px; }
.section-count { background: #1e2f47; color: #bfdbfe; font-size: 10px; min-width: 20px; text-align: center; border-radius: 10px; padding: 2px 5px; }
.chevron { margin-left: auto; font-size: 17px; }
:global(.section-trigger[data-state='open']) .chevron { transform: rotate(180deg); }
.section-fields { padding: 0 16px 16px; }
.check { font-size: 12px; }
.field-help { font-size: 10px; line-height: 1.6; color: var(--muted); margin: 0 0 6px; }
.soil-type { display: grid; gap: 8px; font-size: 12px; margin: 8px 0; }
.soil-type select { width: 100%; font-size: 12px; }
.section-fields .acreage { margin: 8px 0 12px; }
.validation { font-size: 11px; color: #ffc193; margin: 8px 0; }
.preview-selections { display: flex; flex-wrap: wrap; gap: 6px; padding: 14px 16px; }
.panel-footer { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 8px 16px; border-top: 1px solid var(--border); font-size: 11px; }
.panel-footer .text-button { margin: 0; padding: 4px 0; }
</style>

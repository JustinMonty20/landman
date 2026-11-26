<script lang="ts">
import { onMount } from 'svelte';
import { Dialog, Menu } from '@skeletonlabs/skeleton-svelte';
import '../../style.css';
type State = { id:string; name:string; position:number; revision:number; count:number };
type Card = { favorite?:boolean; parcel_id:string; parcel_number:string|null; address:string|null; state_id:string; revision:number; excluded:boolean; exclusion_reason:string; favorited_at:string };
type Board = { states:State[]; default_state_id:string; revision:number; next:string };
let board = $state<Board>();
let cards = $state<Record<string,Card[]>>({});
let next = $state<Record<string,string>>({});
let busy = $state(false);
type LoadErrors = {board?:string; states?:string; columns:Record<string,string|undefined>};
let loadErrors = $state<LoadErrors>({columns:{}});
let mutationError = $state('');
let loading = $state(true);
let name = $state('');
let hideExcluded = $state(false);
class RequestError extends Error {
 status:number;
 constructor(status:number){super(status===409 ? 'Another tab changed this record. Reload and retry.' : `Request failed (${status}).`);this.status=status}
}
async function api<T>(url:string, method='GET', payload?:object):Promise<T> {
 const response = await fetch(url,{ method, headers:payload ? {'Content-Type':'application/json'} : undefined, body:payload ? JSON.stringify(payload) : undefined });
 if (!response.ok) throw new RequestError(response.status);
 return response.json();
}
let generation = 0;
let pendingCards = $state<Record<string,boolean>>({});
let pendingStates = $state(false);
async function load() {
const request=++generation;loading=true;loadErrors={columns:{}};pendingStates=false;pendingCards={};next={};cards={};board=undefined;
try {const data=await api<Board>('/api/me/crm/board');if(request!==generation)return;board=data;await Promise.all(data.states.map(s=>loadCards(s.id,false,request)))}
catch(e){if(request===generation)loadErrors.board=String(e)}finally{if(request===generation)loading=false}
}
async function loadMoreStates(){
if(!board?.next || pendingStates)return;
const request=generation,cursor=board.next;pendingStates=true;loadErrors.states=undefined;
try{const page=await api<Board>(`/api/me/crm/board?cursor=${encodeURIComponent(cursor)}`);if(request!==generation || board?.next!==cursor)return;board={...board,states:[...board.states,...page.states],next:page.next};await Promise.all(page.states.map(s=>loadCards(s.id,false,request)))}
catch(e){if(request===generation && board?.next===cursor)loadErrors.states=String(e)}finally{if(request===generation)pendingStates=false}
}
async function loadCards(id:string,more:boolean,request=generation) {
if(pendingCards[id])return;
const cursor=more?next[id]:'';
if(more && !cursor)return;
const hide=hideExcluded;
pendingCards[id]=true;loadErrors.columns[id]=undefined;
try {const params=new URLSearchParams({limit:'20',hide_excluded:String(hide)});if(cursor)params.set('cursor',cursor);
const data=await api<{items:Card[];next:string}>(`/api/me/crm/board/${id}?${params}`);
if(request!==generation || hide!==hideExcluded || (more && next[id]!==cursor))return;
cards[id]=more?[...(cards[id]??[]),...data.items]:data.items;next[id]=data.next;loadErrors.columns[id]=undefined;
}catch(e){if(request===generation && hide===hideExcluded && (!more || next[id]===cursor))loadErrors.columns[id]=String(e)}
finally{if(request===generation)pendingCards[id]=false}
}
type Action = {kind:'create'} | {kind:'rename'|'delete'|'default'|'left'|'right'; state:State} | {kind:'exclude'|'remove'|'restore'; card:Card};
let action = $state<Action | null>(null);
let dialogOpen = $state(false);
let dialogGeneration=0;
let recovering = $state(false);
function closeAction(){dialogGeneration++;dialogOpen=false;action=null;busy=false;recovering=false;mutationError='';}
let destination = $state('');
let selectedTarget = $state<State|null>(null);
type Freshness = 'valid' | 'refreshing' | 'failed' | 'missing';
let sourceFreshness = $state<Freshness>('valid');
let destinationFreshness = $state<Freshness>('valid');
let deletedDestinations = $state<Set<string>>(new Set());
let unverifiedDestination = $state('');
const destinationOptions = $derived(board?.states.filter(s=>s.id!==(action && 'state' in action ? action.state.id : '') && !deletedDestinations.has(s.id)) ?? []);
const visibleDestinations = $derived(selectedTarget && !deletedDestinations.has(selectedTarget.id) && !destinationOptions.some(s=>s.id===selectedTarget?.id) ? [...destinationOptions,selectedTarget] : destinationOptions);
let reason = $state('');
const reasonBytes = $derived(new TextEncoder().encode(reason.trim()).length);
let dragged = $state('');
let dropTarget = $state('');
function openAction(nextAction:Action) {
 dialogGeneration++;
 action=nextAction; name=nextAction.kind==='rename'?nextAction.state.name:'';reason='';
 selectedTarget=board?.states.find(s=>s.id!==('state' in nextAction?nextAction.state.id:'')) ?? null;
 destination=selectedTarget?.id ?? '';sourceFreshness='valid';destinationFreshness=selectedTarget?'valid':'missing';deletedDestinations=new Set();unverifiedDestination='';
 mutationError='';dialogOpen=true;
}
async function change(url:string,method:string,payload:object):Promise<boolean> {
 const owner=dialogGeneration;
 const owned=()=>dialogGeneration===owner;
 const pendingAction=action, selectedDestination=destination;
 busy=true;recovering=false;mutationError='';
 try {await api(url,method,payload);if(!owned())return false;await load();return owned()}
 catch(e){
  if(!owned())return false;
  recovering=true;
  if(pendingAction && ('state' in pendingAction || 'card' in pendingAction))sourceFreshness='refreshing';
  if(pendingAction?.kind==='delete' && e instanceof RequestError && e.status===409)destinationFreshness='refreshing';
  await load();
  if(!owned() || (pendingAction && (!dialogOpen || action!==pendingAction)))return false;
  if(pendingAction && 'state' in pendingAction){
   try {
    const current=await api<State>(`/api/me/crm/states/${pendingAction.state.id}`);
    if(!owned() || !dialogOpen || action!==pendingAction)return false;
    if(current.id!==pendingAction.state.id || !Number.isSafeInteger(current.revision))throw Error('Invalid source state.');
    action={...pendingAction,state:current};sourceFreshness='valid';
    if(pendingAction.kind==='delete' && selectedDestination && e instanceof RequestError && e.status===409){
     try {
      const target=await api<State>(`/api/me/crm/states/${selectedDestination}`);
      if(!owned() || !dialogOpen || action?.kind!=='delete' || action.state.id!==current.id)return false;
      if(target.id!==selectedDestination || target.id===current.id)throw Error('Invalid destination state.');
      selectedTarget=target;destination=selectedDestination;destinationFreshness='valid';unverifiedDestination='';
     }catch(targetError){
      if(!owned() || !dialogOpen || action?.kind!=='delete')return false;
      if(targetError instanceof RequestError && targetError.status===404){deletedDestinations=new Set([...deletedDestinations,selectedDestination]);selectedTarget=null;destination='';destinationFreshness='missing';mutationError='Destination state was deleted. Select another state.'}
      else {destinationFreshness='failed';unverifiedDestination=selectedDestination;mutationError=`Destination state could not be refreshed (${String(targetError)}). Select another state or cancel and reopen the action.`}
      return false;
     }
    }
   }catch(readError){if(!owned() || !dialogOpen)return false;sourceFreshness=readError instanceof RequestError && readError.status===404?'missing':'failed';mutationError=`${String(e)} Target state could not be refreshed (${String(readError)}). Cancel and reopen the action.`;return false}
  } else if(pendingAction && 'card' in pendingAction){
   try {const current=await api<Card>(`/api/me/favorites/${pendingAction.card.parcel_id}`);
    if(!owned())return false;
    if(!current.favorite){sourceFreshness='missing';mutationError=`${String(e)} This parcel is no longer a favorite. Cancel and reopen the action.`;return false}
    action={...pendingAction,card:current};sourceFreshness='valid';
   }catch(readError){if(!owned())return false;sourceFreshness=readError instanceof RequestError && readError.status===404?'missing':'failed';mutationError=`${String(e)} Target parcel could not be refreshed (${String(readError)}). Cancel and reopen the action.`;return false}
  }
  mutationError=String(e);return false;
 }finally{if(owned()){busy=false;recovering=false}}
}
function move(card:Card,id:string) {if(id!==card.state_id)void change(`/api/me/favorites/${card.parcel_id}/state`,'PUT',{state_id:id,revision:card.revision})}
function drop(event:DragEvent,id:string) {event.preventDefault();dropTarget='';const parcel=event.dataTransfer?.getData('text/plain');const card=Object.values(cards).flat().find(c=>c.parcel_id===parcel);if(card)move(card,id);dragged=''}
async function submitAction() {
 if (!action || busy || !board || sourceFreshness!=='valid') return;
 const submitted=action, owner=dialogGeneration;
 let ok=false;
 if(submitted.kind==='create' && name.trim()) ok=await change('/api/me/crm/states','POST',{name:name.trim()});
 if('state' in submitted){const s=submitted.state;
  if(submitted.kind==='rename' && name.trim()) ok=await change(`/api/me/crm/states/${s.id}`,'PATCH',{name:name.trim(),revision:s.revision});
  if(submitted.kind==='delete' && destination && destinationFreshness==='valid' && !deletedDestinations.has(destination) && destination!==s.id) ok=await change(`/api/me/crm/states/${s.id}`,'DELETE',{destination_id:destination,revision:s.revision});
  if(submitted.kind==='default') ok=await change('/api/me/crm/settings','PUT',{state_id:s.id,revision:board.revision});
  if(submitted.kind==='left'||submitted.kind==='right') ok=await change(`/api/me/crm/states/${s.id}`,'PATCH',{position:Math.max(0,s.position+(submitted.kind==='left'?-1:1)),revision:s.revision});
 }
 if('card' in submitted){const c=submitted.card;
  if(submitted.kind==='exclude' && reason.trim() && reasonBytes<=500) ok=await change(`/api/me/favorites/${c.parcel_id}/exclusion`,'PUT',{reason:reason.trim(),revision:c.revision});
  if(submitted.kind==='restore') ok=await change(`/api/me/favorites/${c.parcel_id}/exclusion`,'DELETE',{revision:c.revision});
  if(submitted.kind==='remove') ok=await change(`/api/me/favorites/${c.parcel_id}`,'DELETE',{revision:c.revision});
 }
 if(ok && owner===dialogGeneration){closeAction();name='';reason=''}
}
onMount(()=>{void load()});
</script>
<svelte:head><title>Leads | Landman</title></svelte:head>
<main class="favorites">
<div class="intro"><div><p class="eyebrow">YOUR WORKSPACE</p><h1>Leads</h1><p class="subtitle">Your private board for saved parcels and lead states.</p></div></div>
<div class="toolbar"><div class="board-name"><span class="board-mark" aria-hidden="true">▦</span><div><strong>Board</strong><small>One private board · {board?.states.length ?? 0}{board?.next ? '+' : ''} states</small></div></div>
<div class="toolbar-actions"><label class="filter"><input type="checkbox" bind:checked={hideExcluded} onchange={() => void load()} /> Hide excluded</label><button class="btn btn-sm preset-outlined-surface-300-700" onclick={() => void load()} disabled={busy}>↻ <span>Reload board</span></button><button class="btn btn-sm preset-filled-primary-500" onclick={() => openAction({kind:'create'})} disabled={!board || busy}>+ Add state</button></div></div>
{#if loading}<p class="notice" role="status">Loading your board…</p>{/if}
{#if loadErrors.board}<p class="notice error" role="alert">{loadErrors.board} <button class="inline-action" onclick={load}>Retry board load</button></p>{/if}
{#if mutationError && !dialogOpen}<p class="notice error" role="alert">{mutationError}</p>{/if}
{#if board}
<div class="canvas" aria-label="Lead states"><div class="states">
{#each board.states as s (s.id)}
<section class="column" class:drop-active={dropTarget===s.id} aria-label={s.name} ondragover={(e)=>{e.preventDefault();if(dragged)dropTarget=s.id}} ondragleave={(e)=>{if(!e.currentTarget.contains(e.relatedTarget as Node))dropTarget=''}} ondrop={(e)=>drop(e,s.id)}>
<div class="column-head"><span class="state-dot" aria-hidden="true"></span><h2>{s.name}</h2><span class="count" aria-label={`${s.count} favorites`}>{s.count}</span>{#if board.default_state_id===s.id}<span class="default">Default</span>{/if}
<Menu><Menu.Trigger class="icon-button" aria-label={`Actions for ${s.name}`} disabled={busy}>···</Menu.Trigger><Menu.Positioner><Menu.Content class="action-menu">
<Menu.Item value="rename" onclick={() => openAction({kind:'rename',state:s})}>Rename state</Menu.Item>
{#if board.default_state_id!==s.id}<Menu.Item value="default" onclick={() => openAction({kind:'default',state:s})}>Set as default</Menu.Item>{/if}
{#if s.position>0}<Menu.Item value="left" onclick={() => openAction({kind:'left',state:s})}>Move left</Menu.Item>{/if}
{#if s.position<board.states.length-1}<Menu.Item value="right" onclick={() => openAction({kind:'right',state:s})}>Move right</Menu.Item>{/if}
{#if board.states.length>1}<Menu.Separator/><Menu.Item value="delete" onclick={() => openAction({kind:'delete',state:s})}>Delete state…</Menu.Item>{/if}
</Menu.Content></Menu.Positioner></Menu></div>
<div class="column-body">
{#if loadErrors.columns[s.id]}<p class="column-message error" role="alert">{loadErrors.columns[s.id]} <button class="inline-action" onclick={() => void loadCards(s.id,cards[s.id] !== undefined)} disabled={pendingCards[s.id]}>Retry {s.name} cards</button></p>{:else if pendingCards[s.id] && !cards[s.id]}<p class="column-message" role="status">Loading parcels…</p>{:else if !pendingCards[s.id] && !cards[s.id]?.length}<p class="column-message">No {hideExcluded?'visible ':''}favorites in this state.</p>{/if}
{#each cards[s.id]??[] as card (card.parcel_id)}
<article class="card" draggable="true" ondragstart={(e)=>{dragged=card.parcel_id;e.dataTransfer?.setData('text/plain',card.parcel_id)}} ondragend={()=>{dragged='';dropTarget=''}}>
<div class="card-top"><span class="parcel-label">PARCEL</span><Menu><Menu.Trigger class="icon-button" aria-label={`Actions for parcel ${card.parcel_number || card.parcel_id}`} disabled={busy}>···</Menu.Trigger><Menu.Positioner><Menu.Content class="action-menu"><Menu.Item value="exclude" onclick={() => openAction({kind:card.excluded?'restore':'exclude',card})}>{card.excluded?'Restore lead':'Exclude lead…'}</Menu.Item><Menu.Separator/><Menu.Item value="remove" onclick={() => openAction({kind:'remove',card})}>Remove favorite…</Menu.Item></Menu.Content></Menu.Positioner></Menu></div>
<strong class="parcel-id">{card.parcel_number || card.parcel_id}</strong><p class="address">{card.address || 'Address unavailable'}</p>
{#if card.excluded}<p class="exclusion"><span>Excluded</span> · {card.exclusion_reason}</p>{/if}
<div class="card-footer"><a href={`/?parcel=${card.parcel_id}`}>View parcel <span aria-hidden="true">↗</span></a><label class="state-select"><select aria-label={`Change state for ${card.parcel_number || card.parcel_id}`} value={card.state_id} onchange={(e)=>move(card,e.currentTarget.value)} disabled={busy}>{#each board.states as target}<option value={target.id}>{target.name}</option>{/each}</select></label></div>
</article>
{/each}
{#if next[s.id]}<button class="more" disabled={busy || pendingCards[s.id]} onclick={() => void loadCards(s.id,true)}>Load more in {s.name}</button>{/if}
</div></section>
{/each}
</div></div>
{#if loadErrors.states}<p class="notice error" role="alert">{loadErrors.states}</p>{/if}
{#if board.next}<button class="btn preset-outlined-surface-300-700 state-more" disabled={busy || pendingStates} onclick={loadMoreStates}>Load more states</button>{/if}
{/if}
</main>
<Dialog open={dialogOpen} onOpenChange={(details)=>{if(!details.open && (!busy || recovering))closeAction()}}>
<Dialog.Backdrop class="dialog-backdrop"/><Dialog.Positioner class="dialog-positioner"><Dialog.Content class="dialog-panel">
<Dialog.Title class="dialog-title">{action?.kind==='create'?'Add state':action?.kind==='rename'?'Rename state':action?.kind==='delete'?'Delete state':action?.kind==='default'?'Change default state':action?.kind==='left'||action?.kind==='right'?'Reorder state':action?.kind==='exclude'?'Exclude lead':action?.kind==='restore'?'Restore lead':'Remove favorite'}</Dialog.Title>
<Dialog.Description class="dialog-description">{action?.kind==='delete' ? `Move all parcels from ${'state' in (action ?? {}) ? (action as {state:State}).state.name : ''} to another state before deleting. This cannot be undone.` : action?.kind==='remove' ? 'Remove this favorite? Its state and exclusion remain saved if you add it again.' : action?.kind==='exclude' ? 'The parcel stays in its current state. Give a reason for excluding this lead.' : action?.kind==='restore' ? 'Clear this lead’s exclusion and reason?' : action?.kind==='default' ? 'New favorites will start in this state.' : action?.kind==='left'||action?.kind==='right' ? 'Change the order of your board columns?' : 'Use a short name that distinguishes this state.'}</Dialog.Description>
<form onsubmit={(e)=>{e.preventDefault();void submitAction()}}>
{#if action?.kind==='create'||action?.kind==='rename'}<label class="field">State name<input class="input" bind:value={name} required maxlength="80" disabled={busy}/></label>{/if}
{#if action?.kind==='exclude'}<label class="field">Exclusion reason<textarea class="textarea" bind:value={reason} required disabled={busy} aria-describedby="reason-limit"></textarea><span id="reason-limit" class:error={reasonBytes>500}>{reasonBytes} / 500 UTF-8 bytes{reasonBytes>500 ? ' · Shorten the reason before saving.' : ''}</span></label>{/if}
{#if action?.kind==='delete'}<label class="field">Move parcels to<select class="select" bind:value={destination} onchange={() => {selectedTarget=visibleDestinations.find(s=>s.id===destination) ?? null;destinationFreshness=selectedTarget && !deletedDestinations.has(destination) && destination!==unverifiedDestination ? 'valid':'missing';if(sourceFreshness==='valid')mutationError=destination===unverifiedDestination?'Destination state could not be verified. Select another state or cancel and reopen the action.':''}} required disabled={busy}>{#if !destination}<option value="" disabled>Select a state</option>{/if}{#each visibleDestinations as target}<option value={target.id}>{target.name}</option>{/each}</select></label>{/if}
{#if mutationError}<p class="error" role="alert">{mutationError}</p>{/if}
<div class="dialog-actions"><Dialog.CloseTrigger class="btn preset-outlined-surface-300-700" type="button" disabled={busy && !recovering}>Cancel</Dialog.CloseTrigger><button class="btn preset-filled-primary-500" type="submit" disabled={busy || sourceFreshness!=='valid' || (action?.kind==='delete' && (!destination || destinationFreshness!=='valid' || deletedDestinations.has(destination) || destination===action.state.id)) || (action?.kind==='exclude' && reasonBytes>500)}>{busy?'Saving…':action?.kind==='delete'?'Delete state':action?.kind==='remove'?'Remove favorite':action?.kind==='exclude'?'Exclude lead':'Save changes'}</button></div>
</form></Dialog.Content></Dialog.Positioner></Dialog>
<style>
.favorites{min-height:100dvh;padding:24px clamp(20px,4vw,64px) 48px;background:var(--color-surface-950);color:var(--color-surface-50)}
.intro{margin-bottom:28px}
.eyebrow{color:var(--color-primary-300);font-size:10px;font-weight:800;letter-spacing:.17em;margin:0 0 8px}
.intro h1{font-size:clamp(28px,3vw,38px);font-weight:720;letter-spacing:-.04em;line-height:1.15}
.subtitle{color:var(--muted);font-size:13px;margin:9px 0 0}
.toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:14px 18px;background:var(--color-surface-900);border:1px solid var(--border);border-radius:10px 10px 0 0}
.board-name{display:flex;align-items:center;gap:11px}
.board-mark{display:grid;place-items:center;color:var(--color-primary-300);background:#293c52;width:34px;height:34px;border-radius:8px}
.board-name strong{display:block;font-size:14px}
.board-name small{display:block;color:var(--muted);font-size:11px;margin-top:2px}
.toolbar-actions{display:flex;align-items:center;gap:12px;flex-wrap:wrap}
.filter{display:flex;align-items:center;gap:7px;color:var(--muted);font-size:12px;min-height:40px;white-space:nowrap}
.filter input{accent-color:var(--accent)}
.toolbar button{min-height:38px}
.canvas{overflow-x:auto;background:#292d32;border:1px solid var(--border);border-top:0;border-radius:0 0 10px 10px;padding:20px;scrollbar-color:var(--color-surface-600) transparent}
.states{display:flex;align-items:stretch;gap:14px;min-height:440px;width:max-content}
.column{width:290px;flex:none;background:var(--color-surface-900);border:1px solid #464c54;border-radius:9px;transition:background .15s,border-color .15s}
.column.drop-active{border-color:var(--accent);background:#304054}
.column-head{height:52px;display:flex;align-items:center;gap:8px;padding:0 11px;border-bottom:1px solid #464c54}
.state-dot{height:8px;width:8px;background:var(--color-primary-400);border-radius:50%;flex:none}
.column:nth-child(3n+2) .state-dot{background:#b7a3f4}
.column:nth-child(3n) .state-dot{background:#e8b673}
.column h2{font-size:13px;font-weight:700;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.count{font-size:11px;color:var(--muted);background:#414750;border-radius:12px;padding:2px 7px}
.default{font-size:10px;color:var(--color-primary-300)}
:global(.icon-button){margin-left:auto;min-width:36px;min-height:38px;border-radius:6px;color:var(--muted);font-size:22px;line-height:1;cursor:pointer}
:global(.icon-button:hover),.more:hover{background:#454e58;color:white}
.column-body{padding:10px;display:grid;align-content:start;gap:9px}
.card{padding:13px;background:#272b30;border:1px solid #444a51;border-radius:8px;box-shadow:0 2px 8px #0002;overflow-wrap:anywhere}
.card:hover{border-color:#747f8a}
.card-top,.card-footer{display:flex;align-items:center;justify-content:space-between;gap:8px}
.card-top{height:26px}
.parcel-label{font-size:9px;color:var(--muted);font-weight:800;letter-spacing:.14em}
.parcel-id{display:block;color:#f2f4f6;font-size:15px;letter-spacing:.01em;margin:6px 0 3px}
.address{color:var(--muted);font-size:12px;min-height:32px;line-height:1.4}
.exclusion{font-size:11px;color:#f4c896;margin:10px 0 0}
.exclusion span{font-weight:750}
.card-footer{border-top:1px solid #454b52;padding-top:9px;margin-top:12px}
.card-footer a{color:var(--color-primary-300);font-size:11px;text-decoration:none;white-space:nowrap}
.card-footer a:hover{text-decoration:underline}
.state-select select{max-width:115px;min-height:36px;background:#394149;border:0;border-radius:5px;color:var(--color-surface-50);font-size:11px;padding:0 6px;cursor:pointer}
.column-message{padding:22px 12px;color:var(--muted);font-size:12px;line-height:1.5}
.error{color:#ffbcb8}
.more{font-size:12px;color:var(--color-primary-300);padding:10px;border-radius:5px;min-height:40px}
.notice{padding:12px 16px;background:var(--surface);font-size:13px}
.state-more{margin-top:14px}
:global(.action-menu){z-index:30;min-width:170px;background:#3b4046;border:1px solid #68727c;border-radius:8px;padding:5px;box-shadow:0 15px 32px #0009;color:#f2f4f6}
:global(.action-menu [role=menuitem]){padding:9px 11px;border-radius:5px;font-size:12px;cursor:pointer}
:global(.action-menu [role=menuitem]:hover),:global(.action-menu [role=menuitem][data-highlighted]){background:#4b5868}
:global(.action-menu [role=separator]){border-top:1px solid #68727c;margin:4px}
:global(.dialog-backdrop){position:fixed;inset:0;background:#080b0dcc;z-index:40}
:global(.dialog-positioner){position:fixed;inset:0;z-index:41;display:grid;place-items:center;padding:20px}
:global(.dialog-panel){width:min(440px,100%);background:var(--color-surface-900);border:1px solid #69727c;border-radius:12px;padding:26px;box-shadow:0 25px 70px #000a}
:global(.dialog-title){font-size:20px;font-weight:700}
:global(.dialog-description){font-size:13px;line-height:1.5;color:var(--muted);margin:10px 0 20px}
.field{display:grid;gap:7px;font-size:12px;font-weight:650}
.field input,.field select,.field textarea{width:100%;background:var(--color-surface-950);color:white;border:1px solid var(--border);border-radius:6px;padding:10px}
.field textarea{min-height:100px}
.dialog-actions{display:flex;justify-content:flex-end;gap:10px;margin-top:22px}
.dialog-actions button{min-height:40px}@media(max-width:800px){.favorites{padding:64px 16px 24px}
.toolbar{align-items:flex-start;flex-direction:column}
.toolbar-actions{width:100%;gap:8px}
.canvas{padding:12px}
.column{width:280px}}@media(prefers-reduced-motion:reduce){.column{transition:none}}
</style>

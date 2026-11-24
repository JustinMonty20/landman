<script lang="ts">
import { onMount } from 'svelte';
import '../../style.css';
type State = { id:string; name:string; position:number; revision:number; count:number };
type Card = { parcel_id:string; parcel_number:string|null; address:string|null; state_id:string; revision:number; excluded:boolean; exclusion_reason:string; favorited_at:string };
type Board = { states:State[]; default_state_id:string; revision:number; next:string };
let board = $state<Board>();
let cards = $state<Record<string,Card[]>>({});
let next = $state<Record<string,string>>({});
let busy = $state(false);
let error = $state('');
let loading = $state(true);
let name = $state('');
let hideExcluded = $state(false);
async function api<T>(url:string, method='GET', payload?:object):Promise<T> {
 const response = await fetch(url,{ method, headers:payload ? {'Content-Type':'application/json'} : undefined, body:payload ? JSON.stringify(payload) : undefined });
 if (!response.ok) throw new Error(response.status===409 ? 'Another tab changed this record. Reload and retry.' : `Request failed (${response.status}).`);
 return response.json();
}
let generation = 0;
let pendingCards = $state<Record<string,boolean>>({});
let pendingStates = $state(false);
async function load() {
const request=++generation;loading=true;error='';pendingCards={};next={};cards={};
try {const data=await api<Board>('/api/me/crm/board');if(request!==generation)return;board=data;await Promise.all(data.states.map(s=>loadCards(s.id,false,request)))}
catch(e){if(request===generation)error=String(e)}finally{if(request===generation)loading=false}
}
async function loadMoreStates(){
if(!board?.next || pendingStates)return;
const request=generation,cursor=board.next;pendingStates=true;
try{const page=await api<Board>(`/api/me/crm/board?cursor=${encodeURIComponent(cursor)}`);if(request!==generation || board?.next!==cursor)return;board={...board,states:[...board.states,...page.states],next:page.next};await Promise.all(page.states.map(s=>loadCards(s.id,false,request)))}
catch(e){if(request===generation)error=String(e)}finally{pendingStates=false}
}
async function loadCards(id:string,more:boolean,request=generation) {
if(pendingCards[id])return;
const cursor=more?next[id]:'';
if(more && !cursor)return;
const hide=hideExcluded;
pendingCards[id]=true;
try {const params=new URLSearchParams({limit:'20',hide_excluded:String(hide)});if(cursor)params.set('cursor',cursor);
const data=await api<{items:Card[];next:string}>(`/api/me/crm/board/${id}?${params}`);
if(request!==generation || hide!==hideExcluded || (more && next[id]!==cursor))return;
cards[id]=more?[...(cards[id]??[]),...data.items]:data.items;next[id]=data.next;if(more)error='';
}catch(e){if(request===generation && hide===hideExcluded && (!more || next[id]===cursor))error=String(e)}
finally{if(request===generation)pendingCards[id]=false}
}
async function change(url:string,method:string,payload:object) {busy=true;error='';try {await api(url,method,payload);await load()}catch(e){error=String(e);await load().catch(()=>{});error=String(e)}finally{busy=false}}
function move(card:Card,id:string) {if(id!==card.state_id)void change(`/api/me/favorites/${card.parcel_id}/state`,'PUT',{state_id:id,revision:card.revision})}
function drop(event:DragEvent,id:string) {event.preventDefault();const parcel=event.dataTransfer?.getData('text/plain');const card=Object.values(cards).flat().find(c=>c.parcel_id===parcel);if(card)move(card,id)}
function addState(){const value=name.trim();if(value){void change('/api/me/crm/states','POST',{name:value}).then(()=>name='')}}
function rename(s:State){const value=prompt('New state name',s.name);if(value!==null)void change(`/api/me/crm/states/${s.id}`,'PATCH',{name:value,revision:s.revision})}
function remove(s:State){const destination=board?.states.find(item=>item.id!==s.id);if(!destination)return;const chosen=prompt(`Move cards in ${s.name} to which state? Options: ${board?.states.filter(item=>item.id!==s.id).map(item=>item.name).join(', ')}`,destination.name);const target=board?.states.find(item=>item.id!==s.id && item.name.toLowerCase()===chosen?.trim().toLowerCase());if(target&&confirm(`Delete ${s.name} and move its cards to ${target.name}?`))void change(`/api/me/crm/states/${s.id}`,'DELETE',{destination_id:target.id,revision:s.revision})}
onMount(()=>{void load()});
</script>
<svelte:head><title>Favorites | Landman</title></svelte:head>
<main class="favorites">
<header><a href="/">▧ landman</a><h1>Favorites board</h1></header>
<p>Private saved parcels. Removing a favorite hides its card but retains its state.</p>
<label><input type="checkbox" bind:checked={hideExcluded} onchange={() => void load()} /> Hide excluded</label>
<button onclick={() => void load()} disabled={busy}>Reload board</button>
{#if loading}<p role="status">Loading favorites…</p>{/if}
{#if error}<p role="alert">{error}</p>{/if}
{#if board}
<div class="states" aria-label="Lead states">
{#each board.states as s (s.id)}
<section class="column" aria-label={s.name} ondragover={(e)=>e.preventDefault()} ondrop={(e)=>drop(e,s.id)}>
<h2>{s.name} <small>{s.count}</small></h2>
<div class="tools"><button onclick={() => rename(s)} disabled={busy}>Rename {s.name}</button><button onclick={() => remove(s)} disabled={busy || board?.states.length<2}>Delete {s.name}</button>
<button onclick={() => void change('/api/me/crm/settings','PUT',{state_id:s.id,revision:board?.revision})} disabled={busy || board.default_state_id===s.id}>{board.default_state_id===s.id?'Default':'Set default'}</button>
<button onclick={() => void change(`/api/me/crm/states/${s.id}`,'PATCH',{position:Math.max(0,s.position-1),revision:s.revision})} disabled={busy || s.position===0}>Move {s.name} left</button></div>
{#if !cards[s.id]?.length}<p>No {hideExcluded?'visible ':''}favorites in this state.</p>{/if}
{#each cards[s.id]??[] as card (card.parcel_id)}
<article class="card" draggable="true" ondragstart={(e)=>e.dataTransfer?.setData('text/plain',card.parcel_id)}>
<strong>{card.parcel_number || card.parcel_id}</strong><p>{card.address || 'Address unavailable'}</p>
{#if card.excluded}<p>Excluded: {card.exclusion_reason}</p>{/if}
<a href={`/?parcel=${card.parcel_id}`}>View parcel context</a>
<label>Change state <select aria-label={`Change state for ${card.parcel_number || card.parcel_id}`} value={card.state_id} onchange={(e)=>move(card,e.currentTarget.value)} disabled={busy}>{#each board.states as target}<option value={target.id}>{target.name}</option>{/each}</select></label>
{#if card.excluded}<button onclick={() => void change(`/api/me/favorites/${card.parcel_id}/exclusion`,'DELETE',{revision:card.revision})} disabled={busy}>Restore lead</button>{:else}<button onclick={() => {const reason=prompt('Why exclude this lead?');if(reason?.trim())void change(`/api/me/favorites/${card.parcel_id}/exclusion`,'PUT',{reason:reason.trim(),revision:card.revision})}} disabled={busy}>Exclude lead</button>{/if}
<button onclick={() => {if(confirm('Remove this favorite? Its state and exclusion remain saved.'))void change(`/api/me/favorites/${card.parcel_id}`,'DELETE',{revision:card.revision})}} disabled={busy}>Remove favorite</button>
</article>
{/each}
{#if next[s.id]}<button disabled={busy || pendingCards[s.id]} onclick={() => void loadCards(s.id,true)}>Load more in {s.name}</button>{/if}
</section>
{/each}
</div>
{#if board.next}<button disabled={busy} onclick={loadMoreStates}>Load more states</button>{/if}
<form onsubmit={(e)=>{e.preventDefault();addState()}}><label>New state <input bind:value={name} maxlength="80" required /></label><button disabled={busy}>Add state</button></form>
{/if}
</main>
<style>
.favorites{padding:24px;background:#24272b;color:#eee;min-height:100dvh}.favorites header{display:flex;gap:30px;align-items:center}.favorites a{color:#93c5fd}.favorites button,.favorites input,.favorites select{min-height:36px;margin:4px;padding:5px;background:#303438;color:#eee;border:1px solid #88939c;border-radius:4px}.states{display:flex;gap:14px;overflow-x:auto;margin:20px 0;align-items:flex-start}.column{flex:0 0 min(300px,85vw);padding:12px;background:#303438;border:1px solid #64717d;border-radius:8px;min-height:260px}.tools{display:flex;flex-wrap:wrap}.card{padding:12px;background:#24272b;border:1px solid #64717d;margin:12px 0;display:grid;gap:7px}.card p{margin:0;overflow-wrap:anywhere}.card select{max-width:100%}
</style>

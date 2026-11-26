<script lang="ts">
import { onMount } from 'svelte';
type Note = { id:string; user_id:string; parcel_id:string; body:string; created_at:string; updated_at:string; revision:number };
type Page = { items:Note[]; next:string };
let { parcel, mode = 'preview', drafts, edits, retries }: {parcel:string; mode?:'preview'|'timeline'; drafts:Record<string,string>; edits:Record<string,string>; retries:Record<string,{id:string;body:string}>} = $props();
let items = $state<Note[]>([]);
let next = $state('');
let loading = $state(true);
let pending = $state(false);
let error = $state('');
let conflict = $state(false);
let savedBeyondPage = $state(false);
let editing = $state('');
let deleting = $state('');
let active = true;
const path = $derived(`/api/me/favorites/${parcel}/notes`);
const large = (body:string) => new TextEncoder().encode(body).length > 4000;
const preview = (body:string) => body.length > 160 ? `${body.slice(0,160)}…` : body;
async function api<T>(url:string,method='GET',payload?:object):Promise<T> {
 const res=await fetch(url,{method,headers:payload?{'Content-Type':'application/json'}:undefined,body:payload?JSON.stringify(payload):undefined});
 if (!res.ok) throw new Error(res.status===409?'conflict':`Request failed (${res.status}).`);
 return res.status===204?undefined as T:res.json();
}
async function load(more=false){
 if (pending || (more && !next)) return;
 loading=true;error='';
 try {
  const cursor=more?next:'';
  const page=await api<Page>(`${path}?limit=${mode==='preview'?3:20}${mode==='timeline'?'&order=asc':''}${cursor?`&cursor=${encodeURIComponent(cursor)}`:''}`);
  if(!active)return;
  items=more?[...items,...page.items]:page.items;
  next=page.next;
  savedBeyondPage=false;
  conflict=false;
 } catch {if(active)error='Notes unavailable. Retry loading.'}
 finally {if(active)loading=false}
}
onMount(()=>{void load();return ()=>{active=false}});
function failed(e:unknown){
 if(!active)return;
 conflict=e instanceof Error && e.message==='conflict';
 error=conflict?'Note changed in another tab. Reload notes before retrying. Your text is preserved.':'Could not save note. Your text is preserved. Retry.';
}
async function create(){
 const body=drafts[parcel]??'';
 if(!body.trim() || large(body) || pending || loading)return;
 const id=retries[parcel]?.body===body?retries[parcel].id:crypto.randomUUID();retries[parcel]={id,body};
 pending=true;error='';
 try {
  const note=await api<Note>(path,'POST',{id,body});
  if(!active)return;
  if(mode==='preview') items=[note,...items.filter(item=>item.id!==id)].slice(0,3);
  else if(!next) items=[...items.filter(item=>item.id!==id),note];
  else savedBeyondPage=true;
  drafts[parcel]='';delete retries[parcel];conflict=false;
 }catch(e){failed(e)}finally{if(active)pending=false}
}
async function save(note:Note){
 const key=`${parcel}|${note.id}`,body=edits[key]??note.body;
 if(!body.trim() || large(body) || pending || loading || conflict)return;
 pending=true;error='';
 try{
  const result=await api<Note>(`${path}/${note.id}`,'PUT',{body,revision:note.revision});
  if(!active)return;
  items=items.map(item=>item.id===note.id?result:item);
  delete edits[key];editing='';
 }catch(e){failed(e)}finally{if(active)pending=false}
}
async function remove(id:string){
 const note=items.find(item=>item.id===id);
 if(!note || pending || loading || conflict)return;
 pending=true;error='';
 try{
  await api(`${path}/${id}`,'DELETE',{revision:note.revision});
  if(!active)return;
  items=items.filter(item=>item.id!==id);deleting='';
 }catch(e){failed(e)}finally{if(active)pending=false}
}
function date(value:string){return new Date(value).toLocaleString()}
</script>
{#snippet icon(kind:'add'|'edit'|'save'|'cancel'|'delete'|'confirm')}
<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" width="20" height="20">
{#if kind==='add'}<path d="M12 5v14M5 12h14" />
{:else if kind==='edit'}<path d="M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L9 17l-4 1 1-4Z" />
{:else if kind==='save'}<path d="M4 3h14l3 3v15H3V3h1Zm3 0v6h10V3M7 21v-8h10v8" />
{:else if kind==='delete'}<path d="M4 7h16M10 11v6m4-6v6M6 7l1 14h10l1-14M9 7V4h6v3" />
{:else if kind==='confirm'}<path d="m4 12 5 5L20 6" />
{:else}<path d="M4 4 20 20M20 4 4 20" />{/if}
</svg>
{/snippet}
<section class="notes" aria-label="Private parcel scratchpad">
 <div class="section-heading"><h2>{mode==='timeline'?'Notes':'Private scratchpad'}</h2>{#if mode==='preview'}<a href={`/notes?parcel=${encodeURIComponent(parcel)}`}>Notes →</a>{/if}</div>
 <form class="new-note" onsubmit={(event)=>{event.preventDefault();void create()}}>
  <label for="new-note">New note</label>
  <textarea id="new-note" class="textarea" rows="2" value={drafts[parcel]??''} oninput={(e)=>drafts[parcel]=e.currentTarget.value} required></textarea>
  {#if large(drafts[parcel]??'')}<span class="length-error" role="alert">Note is too long. Shorten it before saving.</span>{/if}
  <button class="btn btn-icon preset-filled-primary-500 note-icon" type="submit" aria-label="Add note" title="Add note" disabled={pending || loading || !drafts[parcel]?.trim() || large(drafts[parcel]??'')}>{@render icon('add')}</button>
 </form>
 {#if savedBeyondPage}<p role="status">Note saved. Load more notes to see it in chronological order.</p>{/if}
 {#if error}<p role="alert">{error}</p><button class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>void load()}>Reload notes</button>{/if}
 {#if loading}<p role="status">Loading notes…</p>
 {:else if items.length===0 && !error}<p>No notes yet.</p>{/if}
 <ul aria-label="Dated notes">
 {#each items as note (note.id)}
 <li><div class="note-heading"><p class="note-date"><time datetime={note.created_at}>{date(note.created_at)}</time>{#if note.updated_at!==note.created_at} · Edited <time datetime={note.updated_at}>{date(note.updated_at)}</time>{/if}</p>
  <div class="note-actions" role="group" aria-label={`Actions for note ${date(note.created_at)}`}>
  {#if editing===note.id}
   <button form={`edit-form-${note.id}`} class="btn btn-icon preset-filled-primary-500 note-icon" aria-label="Save note" title="Save note" disabled={pending || loading || conflict || !(edits[`${parcel}|${note.id}`]??note.body).trim() || large(edits[`${parcel}|${note.id}`]??note.body)}>{@render icon('save')}</button>
   <button class="btn btn-icon preset-outlined-surface-300-700 note-icon" aria-label="Cancel edit" title="Cancel edit" disabled={pending} onclick={()=>editing=''}>{@render icon('cancel')}</button>
  {:else if deleting===note.id}
   <span class="confirm-label">Delete this note?</span>
   <button class="btn btn-icon preset-filled-primary-500 note-icon" aria-label="Confirm delete note" title="Confirm delete note" disabled={pending || loading || conflict} onclick={()=>void remove(note.id)}>{@render icon('confirm')}</button>
   <button class="btn btn-icon preset-outlined-surface-300-700 note-icon" aria-label="Cancel delete note" title="Cancel delete note" disabled={pending} onclick={()=>deleting=''}>{@render icon('cancel')}</button>
  {:else}
   <button class="btn btn-icon preset-outlined-surface-300-700 note-icon" aria-label="Edit note" title="Edit note" disabled={pending} onclick={()=>{deleting='';editing=note.id}}>{@render icon('edit')}</button>
   <button class="btn btn-icon preset-outlined-surface-300-700 note-icon" aria-label="Delete note" title="Delete note" disabled={pending} onclick={()=>{editing='';deleting=note.id}}>{@render icon('delete')}</button>
  {/if}</div></div>
  {#if editing===note.id}
   <form id={`edit-form-${note.id}`} onsubmit={(event)=>{event.preventDefault();void save(note)}}><label for={`edit-${note.id}`}>Edit note</label><textarea id={`edit-${note.id}`} class="textarea" rows="3" value={edits[`${parcel}|${note.id}`]??note.body} oninput={(e)=>edits[`${parcel}|${note.id}`]=e.currentTarget.value} required></textarea>{#if large(edits[`${parcel}|${note.id}`]??note.body)}<span class="length-error" role="alert">Note is too long. Shorten it before saving.</span>{/if}</form>
  {:else}<p class="note-body">{mode==='preview'?preview(note.body):note.body}</p>{/if}
 </li>{/each}
 </ul>
 {#if next && mode==='timeline'}<button class="btn btn-sm preset-outlined-surface-300-700" disabled={loading || pending} onclick={()=>void load(true)}>Load more notes</button>{/if}
</section>
<style>
.notes{padding:12px;display:grid;gap:10px;font-size:13px}.section-heading{display:flex;align-items:center;justify-content:space-between;gap:12px}.notes h2{font-size:16px;font-weight:650}.section-heading a{color:var(--color-primary-300);font-weight:600;white-space:nowrap}.notes form{display:grid;gap:6px}.new-note{grid-template-columns:1fr auto;align-items:end}.new-note label,.new-note textarea,.length-error{grid-column:1 / -1}.notes textarea{width:100%;min-height:62px}.notes ul{display:grid;gap:8px;list-style:none;padding:0}.notes li{border:1px solid var(--border);border-radius:8px;padding:10px;overflow-wrap:anywhere}.note-heading{display:flex;align-items:center;justify-content:space-between;gap:8px}.note-body{white-space:pre-wrap;margin-top:5px;line-height:1.45}.note-date{color:var(--muted);font-size:11px}.note-actions{display:flex;align-items:center;gap:4px;flex-shrink:0}.confirm-label{font-size:11px;color:var(--color-surface-200)}.notes button{justify-self:start}.note-icon{width:40px;height:40px;display:grid;place-items:center;padding:0;cursor:pointer}.note-icon:focus-visible{outline:2px solid var(--color-primary-400);outline-offset:2px}.length-error{color:#fbbf74}
</style>

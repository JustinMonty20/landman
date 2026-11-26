<script lang="ts">
import { onMount } from 'svelte';
import { Dialog } from '@skeletonlabs/skeleton-svelte';
type Note = { id:string; user_id:string; parcel_id:string; body:string; created_at:string; updated_at:string; revision:number };
type Page = { items:Note[]; next:string };
let { parcel, drafts, edits, retries }: {parcel:string;drafts:Record<string,string>;edits:Record<string,string>;retries:Record<string,{id:string;body:string}>} = $props();
let items = $state<Note[]>([]);
let next = $state('');
let loading = $state(true);
let pending = $state(false);
let error = $state('');
let conflict = $state(false);
let editing = $state('');
let deleting = $state<Note|null>(null);
let active = true;
const path = $derived(`/api/me/favorites/${parcel}/notes`);
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
  const page=await api<Page>(`${path}?limit=20${cursor?`&cursor=${encodeURIComponent(cursor)}`:''}`);
  if(!active)return;
  items=more?[...items,...page.items]:page.items;
  next=page.next;
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
 if(!body.trim() || new TextEncoder().encode(body).length>4000 || pending || loading)return;
 const id=retries[parcel]?.body===body?retries[parcel].id:crypto.randomUUID();retries[parcel]={id,body};
 pending=true;error='';
 try {
  const note=await api<Note>(path,'POST',{id,body});
  if(!active)return;
  items=[note,...items.filter(item=>item.id!==id)];
  drafts[parcel]='';delete retries[parcel];conflict=false;
 }catch(e){failed(e)}finally{if(active)pending=false}
}
async function save(note:Note){
 const body=edits[`${parcel}|${note.id}`]??note.body;
 if(!body.trim() || new TextEncoder().encode(body).length>4000 || pending || loading || conflict)return;
 pending=true;error='';
 try{
  const result=await api<Note>(`${path}/${note.id}`,'PUT',{body,revision:note.revision});
  if(!active)return;
  items=items.map(item=>item.id===note.id?result:item);
  delete edits[`${parcel}|${note.id}`];editing='';
 }catch(e){failed(e)}finally{if(active)pending=false}
}
async function remove(note:Note){
 if(pending || loading || conflict)return;
 pending=true;error='';
 try{
  await api(`${path}/${note.id}`,'DELETE',{revision:note.revision});
  if(!active)return;
  items=items.filter(item=>item.id!==note.id);deleting=null;
 }catch(e){failed(e)}finally{if(active)pending=false}
}
function date(value:string){return new Date(value).toLocaleString()}
</script>
<section class="notes" aria-label="Private parcel scratchpad">
 <h3>Private scratchpad</h3><p>Notes are private to your account. You can add notes without saving a favorite.</p>
 <form onsubmit={(event)=>{event.preventDefault();void create()}}>
  <label for="new-note">New note</label>
  <textarea id="new-note" class="textarea" rows="3" value={drafts[parcel]??''} oninput={(e)=>drafts[parcel]=e.currentTarget.value} required aria-describedby="note-limit"></textarea>
  <span id="note-limit">{new TextEncoder().encode(drafts[parcel]??'').length} / 4000 UTF-8 bytes</span>
  <button class="btn btn-sm preset-filled-primary-500" type="submit" disabled={pending || loading || !drafts[parcel]?.trim() || new TextEncoder().encode(drafts[parcel]??'').length>4000}>{pending?'Saving…':'Add note'}</button>
 </form>
 {#if error}<p role="alert">{error}</p><button class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>void load()}>Reload notes</button>{/if}
 {#if loading}<p role="status">Loading notes…</p>
 {:else if items.length===0 && !error}<p>No notes yet.</p>{/if}
 <ul aria-label="Dated notes">
 {#each items as note (note.id)}
 <li><p class="note-date">Created <time datetime={note.created_at}>{date(note.created_at)}</time>{#if note.updated_at!==note.created_at} · Edited <time datetime={note.updated_at}>{date(note.updated_at)}</time>{/if}</p>
  {#if editing===note.id}
  <form onsubmit={(event)=>{event.preventDefault();void save(note)}}><label for={`edit-${note.id}`}>Edit note</label><textarea id={`edit-${note.id}`} class="textarea" rows="3" value={edits[`${parcel}|${note.id}`]??note.body} oninput={(e)=>edits[`${parcel}|${note.id}`]=e.currentTarget.value} required></textarea><span>{new TextEncoder().encode(edits[`${parcel}|${note.id}`]??note.body).length} / 4000 UTF-8 bytes</span><div class="note-actions"><button class="btn btn-sm preset-filled-primary-500" disabled={pending || loading || conflict || !(edits[`${parcel}|${note.id}`]??note.body).trim() || new TextEncoder().encode(edits[`${parcel}|${note.id}`]??note.body).length>4000}>Save note</button><button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>editing=''}>Cancel edit</button></div></form>
  {:else}<p class="note-body">{note.body}</p><div class="note-actions"><button class="btn btn-sm preset-outlined-surface-300-700" disabled={pending} onclick={()=>editing=note.id}>Edit</button><button class="btn btn-sm preset-outlined-surface-300-700" disabled={pending} onclick={()=>deleting=note}>Delete…</button></div>{/if}
 </li>{/each}
 </ul>
 {#if next}<button class="btn btn-sm preset-outlined-surface-300-700" disabled={loading || pending} onclick={()=>void load(true)}>Load older notes</button>{/if}
</section>
<Dialog open={!!deleting} onOpenChange={(detail)=>{if(!detail.open && !pending)deleting=null}}>
 <Dialog.Backdrop class="note-backdrop"/><Dialog.Positioner class="note-positioner"><Dialog.Content class="note-dialog"><Dialog.Title>Delete note?</Dialog.Title><Dialog.Description>This removes the note permanently.</Dialog.Description><div class="note-actions"><Dialog.CloseTrigger class="btn preset-outlined-surface-300-700" disabled={pending}>Cancel</Dialog.CloseTrigger><button class="btn preset-filled-primary-500" disabled={pending || loading || conflict} onclick={()=>{if(deleting)void remove(deleting)}}>{pending?'Deleting…':'Delete note'}</button></div></Dialog.Content></Dialog.Positioner>
</Dialog>
<style>
.notes{padding:12px;border-top:1px solid var(--border);display:grid;gap:8px;font-size:12px}.notes h3{font-size:15px;font-weight:600}.notes form{display:grid;gap:6px}.notes textarea{width:100%;min-height:72px}.notes ul{display:grid;gap:10px;list-style:none;padding:0}.notes li{border:1px solid var(--border);border-radius:6px;padding:10px;overflow-wrap:anywhere}.note-body{white-space:pre-wrap}.note-date{color:var(--muted);font-size:11px}.note-actions{display:flex;gap:8px;flex-wrap:wrap}.notes button{justify-self:start}:global(.note-backdrop){position:fixed;inset:0;background:#0009;z-index:30}:global(.note-positioner){position:fixed;inset:0;z-index:31;display:grid;place-items:center;padding:16px}:global(.note-dialog){background:var(--color-surface-900);border:1px solid var(--border);border-radius:8px;padding:24px;max-width:400px;width:100%;display:grid;gap:14px;color:var(--color-surface-50)}
</style>

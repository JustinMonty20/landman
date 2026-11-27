<script lang="ts">
import { onMount } from 'svelte';
import { dueCandidates, smsCapacity } from './taskTime';
let {parcel = '', editable = true, refresh = 0, oncreated}: {parcel?: string; editable?: boolean; refresh?: number; oncreated?: (parcel: string) => void} = $props();
type Task = {id:string;parcel_id:string;parcel_number:string|null;message:string;due_at:string;timezone:string;status:'open'|'completed'|'canceled';reminder_state:'queued'|'stubbed'|'canceled'|'missed';revision:number;schedule_version:number};
let items=$state<Task[]>([]), next=$state(''), loading=$state(false), busy=$state(false), error=$state(''), notice=$state('');
let message=$state(''), local=$state(''), offset=$state(''), editing=$state<Task|null>(null), createID=$state('');
let confirmClear=$state<{count:number;snapshot:string}|null>(null), clearing=$state(false);
let conflict=$state(false), currentTask=$state<Task|null>(null), now=$state(Date.now());
let generation=0, reader:AbortController|null=null;
const timezone=Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
const candidates=$derived(dueCandidates(local,timezone));
const capacity=$derived(smsCapacity(message));
const overdue=$derived(items.filter(t=>Date.parse(t.due_at)<now));
const upcoming=$derived(items.filter(t=>Date.parse(t.due_at)>=now));
const path=$derived(parcel?`/api/me/parcels/${parcel}/tasks`:'/api/me/crm/tasks');
function display(t:Task){return new Intl.DateTimeFormat(undefined,{dateStyle:'medium',timeStyle:'short',timeZone:t.timezone}).format(new Date(t.due_at));}
async function load(more=false){
 if(!more)confirmClear=null;
 if(more && (loading || busy || !next))return;
 reader?.abort();reader=new AbortController();const request=++generation,signal=reader.signal;
 const cursor=more?next:'';loading=true;error='';
 try{const q=new URLSearchParams({limit:'20'});if(cursor)q.set('cursor',cursor);
 const res=await fetch(`${path}?${q}`,{signal});if(!res.ok)throw Error(`Tasks unavailable (${res.status}).`);
 const page:{items:Task[];next:string}=await res.json();
 if(request!==generation)return;
 items=more?[...items,...page.items]:page.items;next=page.next;
 }catch(e){if(request===generation && !signal.aborted)error=String(e)}finally{if(request===generation){loading=false;reader=null}}
}
function edit(t:Task){editing=t;conflict=false;currentTask=null;error='';message=t.message;const date=new Date(t.due_at);const parts=new Intl.DateTimeFormat('en-CA',{timeZone:timezone,year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hourCycle:'h23'}).formatToParts(date);
 const fields=Object.fromEntries(parts.map(p=>[p.type,p.value]));local=`${fields.year}-${fields.month}-${fields.day}T${fields.hour}:${fields.minute}`;
 offset=dueCandidates(local,timezone).find(c=>Date.parse(c.due_at)===date.getTime())?.due_at ?? '';
}
function reset(){editing=null;conflict=false;currentTask=null;message='';local='';offset='';createID='';}
async function refreshEditing(){
 if(!editing || busy)return;
 const id=editing.id;busy=true;error='';
 try{const res=await fetch(`${path}/${id}`);if(!res.ok)throw Error(`Task could not be refreshed (${res.status}).`);
 const fresh:Task=await res.json();
 if(editing?.id!==id)return;
 if(fresh.status!=='open')throw Error('Task is no longer open. Stop editing and reload tasks.');
 editing=fresh;currentTask=fresh;conflict=false;
 }catch(e){error=String(e)}finally{busy=false}
}
async function save(){if(busy || conflict || capacity.remaining<0 || !message.trim() || !candidates.some(c=>c.due_at===offset))return;
 busy=true;error='';notice='';
 const savingParcel=parcel;
 try {const url=editing?`${path}/${editing.id}/reschedule`:path;
 if(!editing && !createID)createID=crypto.randomUUID();
 const payload=editing?{revision:editing.revision,message,due_at:offset,timezone}:{id:createID,message,due_at:offset,timezone};
 const res=await fetch(url,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});
 if(!res.ok){if(res.status===409 && editing){conflict=true;currentTask=null;throw Error('Task changed in another tab. Refresh the task, review its current version, then retry your draft.');}throw Error(`Task not saved (${res.status}).`);}
 const createdParcel=editing?null:savingParcel;
 reset();notice='Task saved. EventBridge scheduling is stubbed; no SMS will be sent.';
 if(createdParcel)oncreated?.(createdParcel);
 await load();
 }catch(e){error=String(e)}finally{busy=false}
}
async function prepareClear(){
error='';try {const res=await fetch(`${path}/clear`);if(!res.ok)throw Error(`Clear count unavailable (${res.status}).`);const data:{count:number;snapshot:string}=await res.json();confirmClear=data;}catch(e){error=String(e)}
}
async function clearAll(){
if(confirmClear===null || clearing)return;
clearing=true;error='';
try {const res=await fetch(`${path}/clear`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(confirmClear)});
if(!res.ok)throw Error(res.status===409?'Task count changed. Review the current tasks and confirm again.':`Clear failed (${res.status}).`);
const data:{canceled:number}=await res.json();confirmClear=null;notice=`Cleared ${data.canceled} open tasks on this parcel. Pending reminders canceled; completed tasks unchanged.`;await load();
}catch(e){const failure=String(e);await load();error=failure}finally{clearing=false}
}
async function action(t:Task,kind:'complete'|'cancel'|'stub'){
 busy=true;error='';notice='';
 try{const res=await fetch(`/api/me/parcels/${t.parcel_id}/tasks/${t.id}/${kind}`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({revision:t.revision})});
 if(!res.ok)throw Error(res.status===409?'Task or reminder changed. Reload before retrying.':`Action failed (${res.status}).`);
 if(kind==='cancel')confirmClear=null;notice=kind==='stub'?'SMS simulation recorded in API log. Nothing was sent.':kind==='complete'?'Task completed. This did not send SMS.':'Task cleared. Pending reminder canceled; no SMS was sent.';await load();
 }catch(e){error=String(e)}finally{busy=false}
}
onMount(()=>{
  if(parcel && new URL(window.location.href).searchParams.get('tab')==='tasks') {
    const tabs=document.querySelector<HTMLElement>('[aria-label="Parcel evidence views"]');
    if(tabs) tabs.scrollLeft=tabs.scrollWidth-tabs.clientWidth;
  }
  void load();const timer=setInterval(()=>{now=Date.now()},1000);
  return ()=>{clearInterval(timer);generation++;reader?.abort()}
});
$effect(()=>{if(refresh>0)void load()});
</script>
<section class="tasks" aria-label={parcel?'Parcel follow-up tasks':'Follow-up task queue'}>
 <header><h2>{parcel?'Follow-up tasks':'Upcoming and overdue follow-ups'}</h2><button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>void load()} disabled={loading || busy}>Reload tasks</button></header>
 {#if parcel && items.length>0}<button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>void prepareClear()} disabled={busy || loading || clearing}>Clear all tasks…</button>{/if}
{#if confirmClear!==null}<div role="group" aria-label="Confirm clear all tasks"><p>Cancel {confirmClear.count} open tasks on this parcel and their pending reminders? Completed tasks stay completed. If tasks change after confirmation, this action fails and asks you to review again.</p><button type="button" class="btn btn-sm preset-filled-primary-500" onclick={()=>void clearAll()} disabled={clearing || confirmClear.count===0}>Confirm clear all</button><button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>confirmClear=null}>Keep tasks</button></div>{/if}
<p class="stub-note">Personal tasks only. EventBridge scheduling and SMS are stubbed in the API. No SMS is sent. Use “Simulate SMS” on a due task to record a simulated result; this never completes the task.</p>
 {#if parcel && editable}
 <form onsubmit={(e)=>{e.preventDefault();void save()}}>
 <label>Message (single SMS segment)<textarea class="textarea" aria-label="Follow-up message" bind:value={message} oninput={()=>{if(!editing)createID=''}} required disabled={busy}></textarea></label>
 <span class:invalid={capacity.remaining<0}>{capacity.remaining} {capacity.encoding} units remaining</span>
 <label>Due date and time<input class="input" type="datetime-local" bind:value={local} oninput={()=>{offset='';createID=''}} required disabled={busy}/></label>
 {#if local && candidates.length===0}<p role="alert">This local time does not exist in {timezone}. Choose another time.</p>{/if}
 {#if candidates.length>0}<label>Time zone offset ({timezone})<select class="select" bind:value={offset} onchange={()=>{createID=''}} required disabled={busy}>
 <option value="" disabled>Select an offset</option>{#each candidates as candidate}<option value={candidate.due_at}>{candidate.label}</option>{/each}</select></label>{/if}
 {#if conflict}<p role="status">Your draft is preserved. Refresh the task to inspect its current version before retrying.</p><button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>void refreshEditing()} disabled={busy}>Refresh task for retry</button>{/if}
 {#if currentTask}<p role="status">Current task (version {currentTask.schedule_version}): {currentTask.message} · {display(currentTask)}. Review this against your draft before saving.</p>{/if}
 <div class="buttons"><button type="submit" class="btn btn-sm preset-filled-primary-500" disabled={busy || conflict || capacity.remaining<0 || !candidates.some(c=>c.due_at===offset) || !message.trim()}>{editing?'Save new schedule version':'Create task'}</button>{#if editing}<button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={reset}>Stop editing</button>{/if}</div>
 </form>{/if}
 {#if notice}<p role="status">{notice}</p>{/if}{#if error}<p role="alert">{error}</p>{/if}
 {#if loading && items.length===0}<p role="status">Loading tasks…</p>{/if}
 {#if !loading && items.length===0 && !error}<p>No open tasks.</p>{/if}
 {#snippet taskRow(t:Task)}<li>
  {#if !parcel}<a href={`/?parcel=${t.parcel_id}&tab=tasks`}>Parcel {t.parcel_number || t.parcel_id}</a>{/if}
  <p class="message">{t.message}</p><p><strong>{Date.parse(t.due_at)<now?'Overdue':'Upcoming'}</strong> · {display(t)} ({t.timezone}) · version {t.schedule_version}</p>
  <p>SMS: {t.reminder_state==='stubbed'?'simulated, not sent':t.reminder_state==='queued'?'queued, not sent':t.reminder_state==='missed'?'missed, not sent':'canceled, not sent'}</p>
  <div class="buttons">{#if parcel && editable}<button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>edit(t)} disabled={busy}>Edit / snooze</button>{/if}
  <button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>void action(t,'complete')} disabled={busy}>Complete task</button>
  <button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>void action(t,'cancel')} disabled={busy}>Clear task</button>
  {#if editable && Date.parse(t.due_at)<=now && t.reminder_state==='queued'}<button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>void action(t,'stub')} disabled={busy}>Simulate SMS (not sent)</button>{/if}</div>
 </li>{/snippet}
{#if parcel}<ul>{#each items as t (t.id)}{@render taskRow(t)}{/each}</ul>
{:else if items.length}<h3>Overdue</h3><ul>{#each overdue as t (t.id)}{@render taskRow(t)}{/each}</ul>
<h3>Upcoming</h3><ul>{#each upcoming as t (t.id)}{@render taskRow(t)}{/each}</ul>{/if}
 {#if next}<button type="button" class="btn btn-sm preset-outlined-surface-300-700" onclick={()=>void load(true)} disabled={loading || busy}>Load more tasks</button>{/if}
</section>
<style>
.tasks{padding:12px;display:grid;gap:12px;font-size:13px}.tasks header,.buttons{display:flex;flex-wrap:wrap;align-items:center;gap:8px}.tasks header{justify-content:space-between;height:auto;min-height:52px;padding:8px 12px;gap:8px}.tasks h2{font-size:16px;font-weight:650}.tasks h3{font-size:14px;font-weight:650;border-bottom:1px solid var(--border);padding-bottom:6px}.tasks form{display:grid;gap:10px}.tasks label{display:grid;gap:4px}.tasks textarea{min-height:70px}.tasks ul{display:grid;gap:8px;list-style:none;padding:0}.tasks li{border:1px solid var(--border);border-radius:8px;padding:12px;overflow-wrap:anywhere}.tasks li p{margin:6px 0}.tasks .message{white-space:pre-wrap}.tasks .invalid,.tasks [role="alert"]{color:#fbbf74}.tasks .stub-note{color:var(--muted)}.tasks a{color:var(--color-primary-300)}
</style>

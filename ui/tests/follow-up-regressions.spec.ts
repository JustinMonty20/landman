import { expect, test, type Page } from '@playwright/test';
const origin='http://127.0.0.1:5174';
const parcel='33333333-3333-4333-8333-333333333333';
const task='44444444-4444-4444-8444-444444444444';
const state='11111111-1111-4111-8111-111111111111';
const due='2030-12-08T14:00:00Z';
const row=(overrides:Record<string,unknown>={})=>({id:task,parcel_id:parcel,parcel_number:'009845',message:'Original reminder',due_at:due,timezone:'UTC',status:'open',reminder_state:'queued',revision:1,schedule_version:1,...overrides});
async function fixture(page:Page){
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.route('**/*',route=>{const url=new URL(route.request().url());if(url.origin!==origin || /^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
 await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
}
async function board(page:Page){
 await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:state,name:'New',position:0,revision:1,count:1}],default_state_id:state,revision:1,next:''}}));
 await page.route(`**/api/me/crm/board/${state}*`,route=>route.fulfill({json:{items:[{parcel_id:parcel,parcel_number:'009845',address:'10 Fixture Lane',state_id:state,favorite:true,excluded:false,exclusion_reason:'',revision:1,favorited_at:'2026-11-24 12:00:00+00'}],next:''}}));
}
test('conflict preserves draft and requires explicit current-task refresh before retry',async ({page},info)=>{
 await fixture(page);
 await page.route('**/api/coverage*',route=>route.fulfill({json:{counties:[]}}));
 await page.route(`**/api/me/favorites/${parcel}`,route=>route.fulfill({json:{parcel_id:parcel,revision:1,favorite:true,excluded:false,exclusion_reason:'',state_id:state}}));
 await page.route(`**/api/me/favorites/${parcel}/notes*`,route=>route.fulfill({json:{items:[],next:''}}));
 let revision=1,posts=0,reads=0;
 const taskRoute=(route:import('@playwright/test').Route)=>{
  const url=new URL(route.request().url());
  if(url.pathname.endsWith('/reschedule')){
   posts++;const body=route.request().postDataJSON();expect(body.message).toBe('Buyer draft');expect(body.revision).toBe(posts===1?1:2);
   if(posts===1)return route.fulfill({status:409,json:{code:'conflict'}});
   return route.fulfill({json:row({revision:3,message:'Buyer draft'})});
  }
  if(url.pathname.endsWith(`/${task}`)){reads++;revision=2;return route.fulfill({json:row({revision:2,message:'Concurrent change',schedule_version:2})})}
  return route.fulfill({json:{items:[row({revision,message:revision===2?'Concurrent change':'Original reminder',schedule_version:revision})],next:''}});
 };
 await page.route(`**/api/me/favorites/${parcel}/tasks/**`,taskRoute);
 await page.route(`**/api/me/favorites/${parcel}/tasks*`,taskRoute);
 await page.goto(`/?parcel=${parcel}`);await page.getByRole('tab',{name:'Tasks'}).click();
 const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});
 await expect(pane.getByText('Original reminder')).toBeVisible();
 await pane.getByRole('button',{name:'Edit / snooze'}).click();
 await pane.getByRole('textbox',{name:'Follow-up message'}).fill('Buyer draft');
 await pane.getByRole('button',{name:'Save new schedule version'}).click();
 await expect(pane.getByRole('alert')).toContainText('Refresh the task');
 await expect(pane.getByRole('button',{name:'Save new schedule version'})).toBeDisabled();
 await expect(pane.getByRole('textbox',{name:'Follow-up message'})).toHaveValue('Buyer draft');
 await page.getByRole('tab',{name:'Research'}).click();await page.getByRole('tab',{name:'Tasks'}).click();
 await expect(pane.getByRole('textbox',{name:'Follow-up message'})).toHaveValue('Buyer draft');
 await expect(pane.getByRole('button',{name:'Save new schedule version'})).toBeDisabled();
 expect(posts).toBe(1);
 await pane.getByRole('button',{name:'Refresh task for retry'}).click();
 await expect(pane.getByText(/Current task \(version 2\): Concurrent change/)).toBeVisible();
 await expect(pane.getByRole('textbox',{name:'Follow-up message'})).toHaveValue('Buyer draft');
 await expect(pane.getByRole('button',{name:'Save new schedule version'})).toBeEnabled();
 await pane.screenshot({path:info.outputPath('conflict-refreshed-draft.png')});
 await pane.getByRole('button',{name:'Save new schedule version'}).click();
 await expect(pane.getByText('Task saved. EventBridge scheduling is stubbed; no SMS will be sent.')).toBeVisible();
 expect(posts).toBe(2);expect(reads).toBe(1);
});
for(const kind of ['exclude','remove'] as const){
 test(`${kind} refreshes embedded task queue after canceled reminder`,async ({page},info)=>{
  await fixture(page);await board(page);
  let canceled=false,queueReads=0;
  await page.route('**/api/me/crm/tasks*',route=>{queueReads++;return route.fulfill({json:{items:[row({reminder_state:canceled?'canceled':'queued'})],next:''}})});
  const favoriteRoute=(route:import('@playwright/test').Route)=>{
   if(route.request().method()==='GET')return route.fulfill({json:{parcel_id:parcel,favorite:true,revision:1}});
   expect(route.request().method()).toBe(kind==='exclude'?'PUT':'DELETE');
   expect(route.request().postDataJSON().revision).toBe(1);
   canceled=true;return route.fulfill({json:{parcel_id:parcel,favorite:kind==='exclude',excluded:kind==='exclude',revision:2}});
  };
  await page.route(`**/api/me/favorites/${parcel}/exclusion`,favoriteRoute);
  await page.route(`**/api/me/favorites/${parcel}`,favoriteRoute);
  await page.goto('/leads');
  await expect(page.getByText('SMS: queued, not sent')).toBeVisible();
  await page.getByRole('button',{name:'Actions for parcel 009845'}).click();
  await page.getByRole('menuitem',{name:kind==='exclude'?'Exclude lead…':'Remove favorite…'}).click();
  if(kind==='exclude')await page.getByRole('textbox',{name:'Exclusion reason'}).fill('Not pursuing');
  await page.getByRole('button',{name:kind==='exclude'?'Exclude lead':'Remove favorite',exact:true}).click();
  await expect(page.getByText('SMS: canceled, not sent')).toBeVisible();
  expect(queueReads).toBeGreaterThanOrEqual(2);
  await page.getByRole('region',{name:'Follow-up task queue'}).screenshot({path:info.outputPath(`${kind}-canceled-queue.png`)});
 });
}
test('upcoming becomes overdue and simulation becomes available at due time without reload',async ({page},info)=>{
 await page.clock.install({time:new Date('2030-12-08T13:59:50Z')});
 await fixture(page);await board(page);
 let reads=0;
 await page.route('**/api/me/crm/tasks*',route=>{reads++;return route.fulfill({json:{items:[row()],next:''}})});
 await page.goto('/leads');
 const queue=page.getByRole('region',{name:'Follow-up task queue'});
 await expect(queue.getByText('Upcoming', {exact:true})).toBeVisible();
 await expect(queue.getByRole('button',{name:'Simulate SMS (not sent)'})).toHaveCount(0);
 await page.clock.runFor(11_000);
 await expect(queue.getByText('Overdue', {exact:true})).toBeVisible();
 await expect(queue.getByRole('button',{name:'Simulate SMS (not sent)'})).toBeVisible();
 expect(reads).toBe(1);
 await queue.screenshot({path:info.outputPath('due-without-reload.png')});
});
for(const source of ['page','reload'] as const)for(const stale of ['page','error'] as const)test(`action refresh supersedes delayed ${source} ${stale} without restoring stale queue`,async ({page},info)=>{
 await fixture(page);await board(page);
 let reads=0,release!:()=>void,entered!:()=>void;
 const requested=new Promise<void>(resolve=>{entered=resolve});
 const held=new Promise<void>(resolve=>{release=resolve});
 await page.route('**/api/me/crm/tasks*',async route=>{
  reads++;
  if(reads===2){entered();await held;return route.fulfill(stale==='error'?{status:503}:{json:{items:[row({id:'55555555-5555-4555-8555-555555555555',message:'Obsolete page'})],next:''}})}
  return route.fulfill({json:{items:reads===1?[row()]:[],next:reads===1?'next-page':''}});
 });
 await page.route(`**/api/me/favorites/${parcel}/tasks/${task}/complete`,route=>route.fulfill({json:row({status:'completed',revision:2})}));
 await page.goto('/leads');
 const queue=page.getByRole('region',{name:'Follow-up task queue'});
 await expect(queue.getByText('Original reminder')).toBeVisible();
 await queue.getByRole('button',{name:source==='page'?'Load more tasks':'Reload tasks'}).click();
 await requested;
 await expect(queue.getByRole('button',{name:'Load more tasks'})).toBeDisabled();
 await queue.getByRole('button',{name:'Complete task'}).click();
 await expect(queue.getByText('No open tasks.')).toBeVisible();
 release();
 await expect(queue.getByText('Original reminder')).toHaveCount(0);
 await expect(queue.getByText('Obsolete page')).toHaveCount(0);
 await expect(queue.getByRole('alert')).toHaveCount(0);
 expect(reads).toBe(3);
 await queue.screenshot({path:info.outputPath(`action-wins-delayed-${source}-${stale}.png`)});
});

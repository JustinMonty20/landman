import { expect, test } from '@playwright/test';
const origin='http://127.0.0.1:5174';
const parcel='33333333-3333-4333-8333-333333333333';
const task='44444444-4444-4444-8444-444444444444';
const state='11111111-1111-4111-8111-111111111111';
test.use({video:'on'});
test('personal follow-up queue explicitly simulates SMS without completing task',async ({page},info)=>{
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.route('**/*',route=>{const url=new URL(route.request().url());if(url.origin!==origin || /^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
 await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
 await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:state,name:'New',position:0,revision:1,count:1}],default_state_id:state,revision:1,next:''}}));
 await page.route(`**/api/me/crm/board/${state}*`,route=>route.fulfill({json:{items:[{parcel_id:parcel,parcel_number:'009845',address:'10 Fixture Lane',state_id:state,favorite:true,excluded:false,exclusion_reason:'',revision:1,favorited_at:'2026-11-24 12:00:00+00'}],next:''}}));
 let simulated=false; const due=new Date(Date.now()-10*60_000).toISOString();
 await page.route('**/api/me/crm/tasks*',route=>route.fulfill({json:{items:[{id:task,parcel_id:parcel,parcel_number:'009845',message:'Review title with the buyer',due_at:due,timezone:'UTC',status:'open',reminder_state:simulated?'stubbed':'queued',revision:simulated?2:1,schedule_version:1}],next:''}}));
 await page.route(`**/api/me/parcels/${parcel}/tasks/${task}/stub`,route=>{expect(route.request().postDataJSON()).toEqual({revision:1});simulated=true;return route.fulfill({json:{id:task,status:'open',reminder_state:'stubbed',revision:2}})});
 await page.goto('/tasks');
 await expect(page.getByRole('heading',{name:'Tasks',exact:true})).toBeVisible();
 await expect(page.getByText('Review title with the buyer')).toBeVisible();
 await expect(page.getByText('SMS: queued, not sent')).toBeVisible();
 await page.screenshot({path:info.outputPath('01-overdue-queued.png'),fullPage:true});
 await page.getByRole('button',{name:'Simulate SMS (not sent)'}).click();
 await expect(page.getByText('SMS: simulated, not sent')).toBeVisible();
 await expect(page.getByText('SMS simulation recorded in API log. Nothing was sent.')).toBeVisible();
 await expect(page.getByText('Review title with the buyer')).toBeVisible();
 await page.screenshot({path:info.outputPath('02-simulated-open.png'),fullPage:true});
});
test('selected parcel composes a single-segment follow-up with explicit offset',async ({page},info)=>{
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.route('**/*',route=>{const url=new URL(route.request().url());if(url.origin!==origin || /^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
 await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
 await page.route('**/api/coverage*',route=>route.fulfill({json:{counties:[]}}));
 await page.route(`**/api/me/favorites/${parcel}`,route=>route.fulfill({json:{parcel_id:parcel,revision:1,favorite:true,excluded:false,exclusion_reason:'',state_id:state}}));
 await page.route(`**/api/me/favorites/${parcel}/notes*`,route=>route.fulfill({json:{items:[],next:''}}));
 let created:{id:string;message:string;due_at:string;timezone:string}|null=null;
 let firstToken='',attempts=0;
 await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>{
  if(route.request().method()==='POST'){
   const body=route.request().postDataJSON();expect(body.message).toBe('Follow up with buyer');expect(body.id).toMatch(/^[0-9a-f-]{36}$/);expect(body.due_at).toMatch(/[+-]\d\d:\d\d$/);
   attempts++;if(attempts===1){firstToken=body.id;return route.fulfill({status:503})}
   expect(body.id).toBe(firstToken);created=body;
   return route.fulfill({json:{...body,parcel_id:parcel,status:'open',reminder_state:'queued',revision:1,schedule_version:1}});
  }
  return route.fulfill({json:{items:created?[{...created,parcel_id:parcel,parcel_number:'009845',status:'open',reminder_state:'queued',revision:1,schedule_version:1}]:[],next:''}});
 });
 await page.goto(`/?parcel=${parcel}`);await page.getByRole('tab',{name:'Tasks'}).click();
 const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});
 await expect(pane.getByRole('heading',{name:'Follow-up tasks'})).toBeVisible();
 await pane.getByRole('textbox',{name:'Follow-up message'}).fill('Follow up with buyer');
 await pane.getByLabel('Due date',{exact:true}).fill('2030-12-08');
 await pane.getByLabel('Due time',{exact:true}).fill('14:00');
 await page.getByRole('tab',{name:'Research'}).click();
 await page.getByRole('tab',{name:'Evidence',exact:true}).click();
 await expect(pane.getByRole('textbox',{name:'Follow-up message'})).toBeHidden();
 await page.getByRole('tab',{name:'Tasks'}).click();
 await expect(pane.getByRole('textbox',{name:'Follow-up message'})).toHaveValue('Follow up with buyer');
 await expect(pane.getByLabel('Due date',{exact:true})).toHaveValue('2030-12-08');
 await expect(pane.getByLabel('Due time',{exact:true})).toHaveValue('14:00');
 await expect(pane.getByLabel(/Time zone offset/)).toBeVisible();
 const choices=pane.getByLabel(/Time zone offset/);
 await choices.selectOption({index:1});
 await pane.getByRole('button',{name:'Create task'}).click();
 await expect(pane.getByRole('alert')).toContainText('Task not saved');
 await pane.getByRole('button',{name:'Create task'}).click();
 expect(attempts).toBe(2);
 await expect(pane.getByText('Task saved. EventBridge scheduling is stubbed; no SMS will be sent.')).toBeVisible();
 await expect(pane.getByText('Follow up with buyer')).toBeVisible();
 await pane.screenshot({path:info.outputPath('03-parcel-created-queued.png')});
});

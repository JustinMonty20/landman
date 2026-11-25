import { expect, test } from '@playwright/test';
const parcel = '33333333-3333-4333-8333-333333333333';
test('deep-linked parcel offers an idempotent favorite action', async ({page}, info) => {
  await page.emulateMedia({reducedMotion:'reduce'});
  const origin='http://127.0.0.1:5174';
  await page.route('**/*',route=>{
    const url=new URL(route.request().url());
    if(url.origin!==origin || /^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();
    return route.continue();
  });
  await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
  await page.route('**/api/me/parcel-detail-fields',route=>route.fulfill({json:{fields:[]}}));
  await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:'11111111-1111-4111-8111-111111111111',name:'New'}],default_state_id:'11111111-1111-4111-8111-111111111111',revision:1,next:''}}));
  let saves=0;
  await page.route(`**/api/me/favorites/${parcel}`,route=>{if(route.request().method()==='GET')return route.fulfill({status:404});saves++;return route.fulfill({json:{parcel_id:parcel,favorite:true,state_id:'11111111-1111-4111-8111-111111111111',revision:1,excluded:false,exclusion_reason:''}})});
  await page.goto(`/?parcel=${parcel}`);
  await expect(page.getByRole('heading',{name:'Parcel evidence'})).toBeVisible();
  await page.getByRole('button',{name:'Save favorite'}).click();
  await expect(page.getByRole('button',{name:'Saved to favorites'})).toBeVisible();
  await expect(page.getByRole('combobox',{name:'Change saved parcel state'})).toHaveValue('11111111-1111-4111-8111-111111111111');
  expect(saves).toBe(1);
  await expect(page.getByRole('link',{name:'Open favorites board'})).toHaveAttribute('href','/favorites');
  await page.screenshot({path:info.outputPath('03-discovery-saved.png'),fullPage:true});
});
test('late save from prior parcel cannot overwrite selected parcel; state move persists across board', async ({page}, info) => {
await page.emulateMedia({reducedMotion:'reduce'});
const origin='http://127.0.0.1:5174', b='44444444-4444-4444-8444-444444444444', newState='22222222-2222-4222-8222-222222222222', oldState='11111111-1111-4111-8111-111111111111';
await page.route('**/*',route=>{const url=new URL(route.request().url());if(url.origin!==origin || /^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
await page.route('**/api/me/parcel-detail-fields',route=>route.fulfill({json:{fields:[]}}));
await page.route('**/api/v1/parcels/lookup*',route=>route.fulfill({json:{items:[{id:b,parcel_number:'B-PARCEL',address:'B Street'}]}}));
let state=oldState,revision=3,release:(()=>void)|undefined;
const card=(id:string)=>({parcel_id:id,favorite:true,state_id:state,revision,excluded:false,exclusion_reason:''});
await page.route(`**/api/me/favorites/${parcel}`,async route=>{
if(route.request().method()==='GET')return route.fulfill({status:404});
await new Promise<void>(resolve=>release=resolve);return route.fulfill({json:{...card(parcel),state_id:newState,revision:99}});
});
await page.route(`**/api/me/favorites/${b}`,route=>route.fulfill({json:card(b)}));
await page.route(`**/api/me/favorites/${b}/state`,route=>{expect(route.request().postDataJSON()).toEqual({state_id:newState,revision:3});state=newState;revision++;return route.fulfill({json:card(b)})});
await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:oldState,name:'New',position:0,revision:1,count:0},{id:newState,name:'Contacted',position:1,revision:1,count:1}],default_state_id:oldState,revision:1,next:''}}));
await page.route('**/api/me/crm/board/*',route=>route.fulfill({json:{items:route.request().url().includes(newState)&&state===newState?[{...card(b),parcel_number:'B-PARCEL',address:'B Street',favorited_at:'2026-11-24'}]:[],next:''}}));
await page.goto(`/?parcel=${parcel}`);await expect(page.getByRole('button',{name:'Save favorite'})).toBeVisible();await page.getByRole('button',{name:'Save favorite'}).click();
await page.getByRole('combobox',{name:'Find parcel ID or address'}).fill('B-PARCEL');await page.getByRole('option',{name:/B Street/}).click();
await expect(page.getByRole('combobox',{name:'Change saved parcel state'})).toHaveValue(oldState);release?.();await expect(page.getByRole('button',{name:'Saved to favorites'})).toBeVisible();await expect(page.getByRole('combobox',{name:'Change saved parcel state'})).toHaveValue(oldState);
await page.getByRole('combobox',{name:'Change saved parcel state'}).selectOption(newState);await expect(page.getByText('State changed.')).toBeVisible();await page.screenshot({path:info.outputPath('04-discovery-state.png'),fullPage:true});
await page.goto(`/?parcel=${b}`);await expect(page.getByRole('combobox',{name:'Change saved parcel state'})).toHaveValue(newState);
await page.goto('/favorites');await expect(page.getByRole('region',{name:'Contacted'}).getByText('B-PARCEL')).toBeVisible();await page.screenshot({path:info.outputPath('05-board-state.png'),fullPage:true});
});
test('failed discovery move restores confirmed selection; conflict refreshes revision', async ({page}) => {
const old='11111111-1111-4111-8111-111111111111', target='22222222-2222-4222-8222-222222222222';
await page.route('**/*',route=>{const url=new URL(route.request().url());if(url.origin!=='http://127.0.0.1:5174' || /^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
await page.route('**/api/me/parcel-detail-fields',route=>route.fulfill({json:{fields:[]}}));
let revision=3,attempts=0;
const card=()=>({parcel_id:parcel,favorite:true,state_id:old,revision,excluded:false,exclusion_reason:''});
await page.route(`**/api/me/favorites/${parcel}`,route=>route.fulfill({json:card()}));
await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:old,name:'New'},{id:target,name:'Contacted'}],default_state_id:old,revision:1,next:''}}));
await page.route(`**/api/me/favorites/${parcel}/state`,route=>{
attempts++;
expect(route.request().postDataJSON()).toEqual({state_id:target,revision:attempts===3?4:3});
if(attempts===2)revision=4;
return route.fulfill({status:attempts===2?409:503,body:'unavailable'});
});
await page.goto(`/?parcel=${parcel}`);
const select=page.getByRole('combobox',{name:'Change saved parcel state'});
await expect(select).toHaveValue(old);
await select.selectOption(target);await expect(page.getByText(/Could not change state/)).toBeVisible();await expect(select).toHaveValue(old);
await select.selectOption(target);await expect(page.getByText(/Another tab changed/)).toBeVisible();await expect(select).toHaveValue(old);
await select.selectOption(target);await expect(page.getByText(/Could not change state/)).toBeVisible();await expect(select).toHaveValue(old);
expect(attempts).toBe(3);
});
test('state 51 is selected and conflict resolves a newly created state by name', async ({page}, info) => {
await page.emulateMedia({reducedMotion:'reduce'});
await page.route('**/*',route=>{const url=new URL(route.request().url());if(url.origin!=='http://127.0.0.1:5174'||/^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
await page.route('**/api/me/parcel-detail-fields',route=>route.fulfill({json:{fields:[]}}));
const states=Array.from({length:52},(_,i)=>({id:`${String(i+1).padStart(8,'0')}-1111-4111-8111-111111111111`,name:`State ${i+1}`}));
let state=states[50],revision=1,attempt=0;
await page.route(`**/api/me/favorites/${parcel}`,route=>route.fulfill({json:{parcel_id:parcel,favorite:true,state_id:state.id,revision,excluded:false,exclusion_reason:''}}));
await page.route('**/api/me/crm/board*',route=>{const cursor=new URL(route.request().url()).searchParams.get('cursor');return route.fulfill({json:{states:cursor?states.slice(50):states.slice(0,50),next:cursor?'':'next',default_state_id:states[0].id,revision:1}})});
await page.route('**/api/me/crm/states/*',route=>route.fulfill({json:state}));
await page.route(`**/api/me/favorites/${parcel}/state`,route=>{attempt++;expect(route.request().postDataJSON()).toEqual({state_id:states[0].id,revision:attempt});if(attempt===1){state=states[51];revision=2;return route.fulfill({status:409})}state=states[0];revision=3;return route.fulfill({json:{parcel_id:parcel,favorite:true,state_id:state.id,revision}})});
await page.goto(`/?parcel=${parcel}`);
const select=page.getByRole('combobox',{name:'Change saved parcel state'});
await expect(select).toHaveValue(states[50].id);await expect(select.locator('option:checked')).toHaveText('State 51');
await page.getByRole('button',{name:'Load more states'}).click();await expect(select.locator('option')).toHaveCount(52);
await select.selectOption(states[0].id);await expect(page.getByText(/Another tab changed/)).toBeVisible();await expect(select).toHaveValue(states[51].id);await expect(select.locator('option:checked')).toHaveText('State 52');
await page.screenshot({path:info.outputPath('state-conflict.png'),fullPage:true});
await select.selectOption(states[0].id);await expect(select.locator('option:checked')).toHaveText('State 1');expect(attempt).toBe(2);
});

const recoveryStates = Array.from({length:3},(_,i)=>({id:`${String(i+61).padStart(8,'0')}-6666-4666-8666-666666666666`,name:`Recovery ${i+1}`}));
async function recoveryPage(page:import('@playwright/test').Page, parcels:string[]){
await page.emulateMedia({reducedMotion:'reduce'});
await page.route('**/*',route=>{const url=new URL(route.request().url());if(url.origin!=='http://127.0.0.1:5174'||/^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
await page.route('**/api/me/parcel-detail-fields',route=>route.fulfill({json:{fields:[]}}));
await page.route('**/api/v1/parcels/lookup*',route=>route.fulfill({json:{items:parcels.map((id,i)=>({id,parcel_number:`RECOVERY-${i}`,address:`Recovery address ${i}`}))}}));
}
const recoveryCard=(id:string,state=recoveryStates[0].id,revision=1)=>({parcel_id:id,favorite:true,state_id:state,revision,excluded:false,exclusion_reason:''});
const recoverySelect=(page:import('@playwright/test').Page)=>page.getByRole('combobox',{name:'Change saved parcel state'});

test('old page 2 success and error cannot lock or alter another parcel pagination',async ({page},info)=>{
const a='aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',b='bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
await recoveryPage(page,[b]);
await page.route(`**/api/me/favorites/${a}`,route=>route.fulfill({json:recoveryCard(a)}));
await page.route(`**/api/me/favorites/${b}`,route=>route.fulfill({json:recoveryCard(b)}));
let release:(()=>void)|undefined,started:(()=>void)|undefined;const requested=new Promise<void>(r=>started=r);let page2=0;
await page.route('**/api/me/crm/board*',async route=>{
const cursor=new URL(route.request().url()).searchParams.get('cursor');
if(cursor){page2++;if(page2===1){started?.();await new Promise<void>(r=>release=r);return route.fulfill({status:503})}return route.fulfill({json:{states:[recoveryStates[2]],next:''}})}
return route.fulfill({json:{states:recoveryStates.slice(0,2),next:'next'}});
});
await page.goto(`/?parcel=${a}`);const more=page.getByRole('button',{name:'Load more states'});
await expect(more).toBeEnabled();await more.click();await requested;
await page.getByRole('combobox',{name:'Find parcel ID or address'}).fill('RECOVERY');await page.getByRole('option',{name:/Recovery address 0/}).click();
await expect(recoverySelect(page)).toHaveValue(recoveryStates[0].id);await expect(more).toBeEnabled();release?.();
await expect(more).toBeEnabled();await expect(page.getByText('Could not load more states. Retry.')).toHaveCount(0);
await more.click();await expect(recoverySelect(page).locator('option')).toHaveCount(3);await expect(recoverySelect(page).locator('option').last()).toHaveText('Recovery 3');expect(page2).toBe(2);
await page.screenshot({path:info.outputPath('recovered-parcel-pagination.png'),fullPage:true});
});

test('pending page 2 superseded by move to loaded state leaves pagination usable',async ({page},info)=>{
const id='cccccccc-cccc-4ccc-8ccc-cccccccccccc';await recoveryPage(page,[]);
let state=recoveryStates[0].id,revision=1,release:(()=>void)|undefined,started:(()=>void)|undefined,page2=0;
const requested=new Promise<void>(r=>started=r);
await page.route(`**/api/me/favorites/${id}`,route=>route.fulfill({json:recoveryCard(id,state,revision)}));
await page.route(`**/api/me/favorites/${id}/state`,route=>{state=recoveryStates[1].id;revision++;return route.fulfill({json:recoveryCard(id,state,revision)})});
await page.route('**/api/me/crm/board*',async route=>{
if(new URL(route.request().url()).searchParams.has('cursor')){page2++;if(page2===1){started?.();await new Promise<void>(r=>release=r);return route.fulfill({json:{states:[{...recoveryStates[2],name:'STALE'}],next:''}})}return route.fulfill({json:{states:[recoveryStates[2]],next:''}})}
return route.fulfill({json:{states:recoveryStates.slice(0,2),next:'next'}});
});
await page.goto(`/?parcel=${id}`);const more=page.getByRole('button',{name:'Load more states'}),select=recoverySelect(page);
await expect(more).toBeEnabled();await more.click();await requested;await select.selectOption(recoveryStates[1].id);
await expect(select).toHaveValue(recoveryStates[1].id);await expect(more).toBeEnabled();release?.();await expect(more).toBeEnabled();
await more.click();await expect(select.locator('option').last()).toHaveText('Recovery 3');expect(page2).toBe(2);
await page.screenshot({path:info.outputPath('recovered-move-pagination.png'),fullPage:true});
});

test('first page failure with confirmed state lookup retries all options',async ({page},info)=>{
const id='dddddddd-dddd-4ddd-8ddd-dddddddddddd';await recoveryPage(page,[]);
await page.route(`**/api/me/favorites/${id}`,route=>route.fulfill({json:recoveryCard(id)}));
let calls=0;await page.route('**/api/me/crm/board',route=>{calls++;return calls===1?route.fulfill({status:503}):route.fulfill({json:{states:recoveryStates,next:''}})});
await page.route(`**/api/me/crm/states/${recoveryStates[0].id}`,route=>route.fulfill({json:recoveryStates[0]}));
await page.goto(`/?parcel=${id}`);const select=recoverySelect(page),retry=page.getByRole('button',{name:'Retry loading states'});
await expect(select).toHaveValue(recoveryStates[0].id);await expect(select.locator('option')).toHaveCount(1);await expect(retry).toBeEnabled();await expect(page.getByRole('alert')).toContainText('Lead states unavailable');
await page.screenshot({path:info.outputPath('first-page-failed.png'),fullPage:true});
await retry.click();await expect(select.locator('option')).toHaveCount(3);await expect(select.locator('option').last()).toHaveText('Recovery 3');await expect(retry).toHaveCount(0);await expect(page.getByRole('alert')).toHaveCount(0);
await page.screenshot({path:info.outputPath('first-page-recovered.png'),fullPage:true});
});

test('board and confirmed state failures keep retry available until both recover',async ({page},info)=>{
const id='eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee';await recoveryPage(page,[]);
await page.route(`**/api/me/favorites/${id}`,route=>route.fulfill({json:recoveryCard(id,recoveryStates[2].id)}));
let board=0,target=0;
await page.route('**/api/me/crm/board',route=>{board++;return board===1?route.fulfill({status:503}):route.fulfill({json:{states:recoveryStates.slice(0,2),next:''}})});
await page.route(`**/api/me/crm/states/${recoveryStates[2].id}`,route=>{target++;return target===1?route.fulfill({status:503}):route.fulfill({json:recoveryStates[2]})});
await page.goto(`/?parcel=${id}`);const select=recoverySelect(page),retry=page.getByRole('button',{name:'Retry loading states'});
await expect(retry).toBeEnabled();await expect(select).toBeDisabled();await expect(page.getByRole('alert')).toHaveCount(2);
await retry.click();await expect(select).toHaveValue(recoveryStates[2].id);await expect(select.locator('option')).toHaveCount(3);await expect(select.locator('option').last()).toHaveText('Recovery 3');await expect(retry).toHaveCount(0);await expect(page.getByRole('alert')).toHaveCount(0);
await page.screenshot({path:info.outputPath('both-recovered.png'),fullPage:true});
});

test('paged confirmed state clears only its recovered lookup error',async ({page},info)=>{
const id='aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee';await recoveryPage(page,[]);
await page.route(`**/api/me/favorites/${id}`,route=>route.fulfill({json:recoveryCard(id,recoveryStates[2].id)}));
await page.route(`**/api/me/crm/states/${recoveryStates[2].id}`,route=>route.fulfill({status:503}));
await page.route('**/api/me/crm/board*',route=>{
const cursor=new URL(route.request().url()).searchParams.get('cursor');
const states=cursor==='last'?recoveryStates.slice(2):cursor?recoveryStates.slice(1,2):recoveryStates.slice(0,1);
return route.fulfill({json:{states,next:cursor==='last'?'':cursor?'last':'middle'}});
});
await page.goto(`/?parcel=${id}`);
const more=page.getByRole('button',{name:'Load more states'}),retry=page.getByRole('button',{name:'Retry loading states'});
await expect(retry).toBeEnabled();await expect(page.getByRole('alert')).toContainText('Confirmed lead state unavailable');
await more.click();await expect(recoverySelect(page).locator('option')).toHaveCount(2);
await expect(retry).toBeEnabled();await expect(page.getByRole('alert')).toContainText('Confirmed lead state unavailable');
await more.click();await expect(recoverySelect(page)).toHaveValue(recoveryStates[2].id);
await expect(recoverySelect(page).locator('option:checked')).toHaveText('Recovery 3');
await expect(page.getByRole('alert')).toHaveCount(0);await expect(retry).toHaveCount(0);
await page.screenshot({path:info.outputPath('paged-lookup-recovered.png'),fullPage:true});
});

test('targeted current-state lookup owns initial loading until it resolves',async ({page})=>{
const id='ffffffff-ffff-4fff-8fff-ffffffffffff';await recoveryPage(page,[]);
await page.route(`**/api/me/favorites/${id}`,route=>route.fulfill({json:recoveryCard(id,recoveryStates[2].id)}));
let release:(()=>void)|undefined,started:(()=>void)|undefined, pages=0;const requested=new Promise<void>(r=>started=r);
await page.route('**/api/me/crm/board*',route=>{if(new URL(route.request().url()).searchParams.has('cursor'))pages++;return route.fulfill({json:{states:recoveryStates.slice(0,2),next:'next'}})});
await page.route(`**/api/me/crm/states/${recoveryStates[2].id}`,async route=>{started?.();await new Promise<void>(r=>release=r);return route.fulfill({json:recoveryStates[2]})});
await page.goto(`/?parcel=${id}`);await requested;
const more=page.getByRole('button',{name:'Load more states'});await expect(more).toBeDisabled();release?.();
await expect(recoverySelect(page)).toHaveValue(recoveryStates[2].id);await expect(more).toBeEnabled();await more.click();expect(pages).toBe(1);
});

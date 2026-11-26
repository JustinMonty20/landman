import {expect,test} from '@playwright/test';
const origin='http://127.0.0.1:5174';
const states=Array.from({length:52},(_,i)=>({id:`${String(i+1).padStart(8,'0')}-1111-4111-8111-111111111111`,name:`State ${i+1}`,position:i,revision:1,count:0}));
async function fixture(page:import('@playwright/test').Page){
 await page.route('**/*',route=>{const url=new URL(route.request().url());return url.origin!==origin || /^\/(api|auth)(\/|$)/.test(url.pathname)?route.abort():route.continue()});
 await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
}
test('delete state 51 conflict resolves selected state 52 by ID and retries with new source revision',async ({page},info)=>{
 await fixture(page);let revision=1,attempts=0,sourceReads=0,targetReads=0;
 await page.route('**/api/me/crm/board*',route=>{const cursor=new URL(route.request().url()).searchParams.has('cursor');return route.fulfill({json:{states:cursor?states.slice(50):states.slice(0,50),next:cursor?'':'cursor',default_state_id:states[0].id,revision:1}})});
 await page.route('**/api/me/crm/board/*',route=>route.fulfill({json:{items:[],next:''}}));
 await page.route(`**/api/me/crm/states/${states[50].id}`,route=>{
  if(route.request().method()==='GET'){sourceReads++;return route.fulfill({json:{...states[50],revision}})}
  expect(route.request().postDataJSON()).toEqual({destination_id:states[51].id,revision});attempts++;
  if(attempts===1){revision=2;return route.fulfill({status:409})}
  return route.fulfill({json:{}});
 });
 await page.route(`**/api/me/crm/states/${states[51].id}`,route=>{targetReads++;return route.fulfill({json:states[51]})});
 await page.goto('/leads');await page.getByRole('button',{name:'Load more states'}).click();await page.getByRole('button',{name:'Actions for State 51'}).click();await page.getByRole('menuitem',{name:'Delete state…'}).click();
 const select=page.getByRole('combobox',{name:'Move parcels to'});await select.selectOption(states[51].id);await page.getByRole('button',{name:'Delete state'}).click();
 await expect(page.getByRole('dialog')).toContainText('Another tab changed');await expect(select).toHaveValue(states[51].id);await expect(select.locator('option:checked')).toHaveText('State 52');await page.screenshot({path:info.outputPath('delete-conflict-state-52.png')});
 await page.getByRole('button',{name:'Delete state'}).click();await expect(page.getByRole('dialog')).toHaveCount(0);expect(attempts).toBe(2);expect(sourceReads).toBe(1);expect(targetReads).toBe(1);
});
test('late destination refresh cannot change a newly opened dialog',async ({page})=>{
 await fixture(page);let release:(()=>void)|undefined,requested=false,sourceReads=0;
 await page.route('**/api/me/crm/board*',route=>{const cursor=new URL(route.request().url()).searchParams.has('cursor');return route.fulfill({json:{states:cursor?states.slice(50):states.slice(0,50),next:cursor?'':'cursor',default_state_id:states[0].id,revision:1}})});
 await page.route('**/api/me/crm/board/*',route=>route.fulfill({json:{items:[],next:''}}));
 await page.route(`**/api/me/crm/states/${states[50].id}`,route=>{if(route.request().method()==='GET'){sourceReads++;return route.fulfill({json:{...states[50],revision:2}})}return route.fulfill({status:409})});
 await page.route(`**/api/me/crm/states/${states[51].id}`,async route=>{requested=true;await new Promise<void>(resolve=>release=resolve);return route.fulfill({status:404})});
 await page.goto('/leads');await page.getByRole('button',{name:'Load more states'}).click();await page.getByRole('button',{name:'Actions for State 51'}).click();await page.getByRole('menuitem',{name:'Delete state…'}).click();await page.getByRole('combobox',{name:'Move parcels to'}).selectOption(states[51].id);await page.getByRole('button',{name:'Delete state'}).click();
 await expect.poll(()=>requested).toBe(true);await expect(page.getByRole('button',{name:'Cancel'})).toBeEnabled();await page.getByRole('button',{name:'Cancel'}).click();await expect(page.getByRole('dialog')).toHaveCount(0);
 await page.getByRole('button',{name:'Add state'}).click();const input=page.getByRole('textbox',{name:'State name'});await input.fill('New buyer state');const settled=page.waitForResponse(response=>response.url().endsWith(`/api/me/crm/states/${states[51].id}`));release?.();await settled;
 await expect(page.getByRole('dialog')).toContainText('Add state');await expect(input).toHaveValue('New buyer state');await expect(page.getByRole('button',{name:'Save changes'})).toBeEnabled();await expect(page.getByRole('dialog').getByRole('alert')).toHaveCount(0);expect(sourceReads).toBe(1);
});
for(const targetFailure of ['deleted','offline'] as const)test(`delete conflict ${targetFailure} destination does not retry stale ID`,async ({page})=>{
 await fixture(page);let writes=0;
 await page.route('**/api/me/crm/board*',route=>{const cursor=new URL(route.request().url()).searchParams.has('cursor');return route.fulfill({json:{states:cursor?states.slice(50):states.slice(0,50),next:cursor?'':'cursor',default_state_id:states[0].id,revision:1}})});
 await page.route('**/api/me/crm/board/*',route=>route.fulfill({json:{items:[],next:''}}));
 await page.route(`**/api/me/crm/states/${states[50].id}`,route=>route.request().method()==='GET'?route.fulfill({json:{...states[50],revision:2}}):(writes++,route.fulfill({status:409})));
 await page.route(`**/api/me/crm/states/${states[51].id}`,route=>targetFailure==='deleted'?route.fulfill({status:404}):route.abort());
 await page.goto('/leads');await page.getByRole('button',{name:'Load more states'}).click();await page.getByRole('button',{name:'Actions for State 51'}).click();await page.getByRole('menuitem',{name:'Delete state…'}).click();
 const select=page.getByRole('combobox',{name:'Move parcels to'});await select.selectOption(states[51].id);await page.getByRole('button',{name:'Delete state'}).click();
 await expect(page.getByRole('dialog')).toContainText(targetFailure==='deleted'?'Destination state was deleted':'Destination state could not be refreshed');
 if(targetFailure==='deleted'){await expect(select).toHaveValue('');await expect(select.locator(`option[value="${states[51].id}"]`)).toHaveCount(0);await expect(page.getByRole('button',{name:'Delete state'})).toBeDisabled();await select.selectOption(states[0].id);await expect(page.getByRole('button',{name:'Delete state'})).toBeEnabled()}
 else {await expect(select).toHaveValue(states[51].id);await expect(page.getByRole('button',{name:'Delete state'})).toBeDisabled()}
 expect(writes).toBe(1);
});
for(const failure of ['abort','invalid-json','http-503'] as const)for(const saved of [true,false])test(`${saved?'DELETE':'PUT'} 409 ${failure} refresh blocks stale mutation until retry read`,async ({page},info)=>{
 await fixture(page);await page.route('**/api/coverage*',route=>route.fulfill({json:{counties:[]}}));
 const id=states[0].id;let reads=0,writes=0,recovered=false;
 const favorite=(revision:number)=>({parcel_id:id,favorite:saved,state_id:id,revision,excluded:false,exclusion_reason:''});
 await page.route(`**/api/me/favorites/${id}`,route=>{
  if(route.request().method()==='GET'){
   reads++;
   if(reads===1)return route.fulfill({json:favorite(3)});
   if(reads===2){if(failure==='abort')return route.abort();if(failure==='invalid-json')return route.fulfill({status:200,body:'{' });return route.fulfill({status:503})}
   recovered=true;return route.fulfill({json:favorite(4)});
  }
  writes++;expect(route.request().method()).toBe(saved?'DELETE':'PUT');expect(route.request().postDataJSON()).toEqual({revision:writes===1?3:4});
  return route.fulfill(writes===1?{status:409}:{json:{...favorite(5),favorite:!saved}});
 });
 await page.goto(`/?parcel=${id}`);const initial=page.getByRole('button',{name:saved?'Remove favorite':'Favorite parcel'});await expect(initial).toBeEnabled();await initial.click();
 const retry=page.getByRole('button',{name:'Retry favorite status'});await expect(retry).toBeEnabled();await expect(retry).toHaveAttribute('title','Retry favorite status');expect(writes).toBe(1);await page.screenshot({path:info.outputPath(`star-retry-${failure}-${saved?'remove':'restore'}.png`)});
 await retry.click();await expect(initial).toBeEnabled();expect(recovered).toBe(true);expect(writes).toBe(1);await initial.click();await expect(page.getByRole('button',{name:saved?'Favorite parcel':'Remove favorite'})).toBeVisible();expect(writes).toBe(2);
});

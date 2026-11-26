import {expect,test} from '@playwright/test';
const origin='http://127.0.0.1:5174';
const states=['A','B','C'].map((name,i)=>({id:`0000000${i+1}-1111-4111-8111-111111111111`,name,position:i,revision:1,count:0}));
async function fixture(page:import('@playwright/test').Page){
 await page.route('**/*',route=>{const url=new URL(route.request().url());return url.origin!==origin || /^\/(api|auth)(\/|$)/.test(url.pathname)?route.abort():route.continue()});
 await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
 await page.route('**/api/me/crm/board*',route=>route.fulfill({json:{states,next:'',default_state_id:states[0].id,revision:1}}));
 await page.route('**/api/me/crm/board/*',route=>route.fulfill({json:{items:[],next:''}}));
}
async function open(page:import('@playwright/test').Page){
 await page.getByRole('button',{name:'Actions for B'}).click();await page.getByRole('menuitem',{name:'Delete state…'}).click();
}
for(const failure of ['503','abort','malformed'] as const)test(`source ${failure} after DELETE 409 cannot be bypassed by selecting C`,async ({page},info)=>{
 await fixture(page);let writes=0,reads=0;
 await page.route(`**/api/me/crm/states/${states[1].id}`,route=>{
  if(route.request().method()==='GET'){reads++;if(reads===1){if(failure==='abort')return route.abort();if(failure==='malformed')return route.fulfill({body:'{',status:200});return route.fulfill({status:503})}return route.fulfill({json:{...states[1],revision:2}})}
  writes++;expect(route.request().postDataJSON()).toEqual({destination_id:states[0].id,revision:1});return route.fulfill({status:409});
 });
 await page.goto('/leads');await open(page);await page.getByRole('button',{name:'Delete state'}).click();
 await expect(page.getByRole('dialog')).toContainText('Target state could not be refreshed');
 await page.getByRole('combobox',{name:'Move parcels to'}).selectOption(states[2].id);
 await expect(page.getByRole('button',{name:'Delete state'})).toBeDisabled();await page.getByRole('dialog').locator('form').evaluate((form:HTMLFormElement)=>form.requestSubmit());
 expect(writes).toBe(1);await page.screenshot({path:info.outputPath(`source-${failure}-blocked.png`)});
 await page.getByRole('button',{name:'Cancel'}).click();await open(page);await expect(page.getByRole('button',{name:'Delete state'})).toBeEnabled();expect(writes).toBe(1);
});
test('stale first-page A is removed after targeted 404; C retry uses refreshed B revision',async ({page},info)=>{
 await fixture(page);let writes=0;
 await page.route(`**/api/me/crm/states/${states[1].id}`,route=>{
  if(route.request().method()==='GET')return route.fulfill({json:{...states[1],revision:2}});
  writes++;expect(route.request().postDataJSON()).toEqual({destination_id:writes===1?states[0].id:states[2].id,revision:writes===1?1:2});return route.fulfill(writes===1?{status:409}:{json:{}});
 });
 await page.route(`**/api/me/crm/states/${states[0].id}`,route=>route.fulfill({status:404}));
 await page.goto('/leads');await open(page);await page.getByRole('button',{name:'Delete state'}).click();
 const select=page.getByRole('combobox',{name:'Move parcels to'});await expect(page.getByRole('dialog')).toContainText('Destination state was deleted');await expect(select.locator(`option[value="${states[0].id}"]`)).toHaveCount(0);await expect(page.getByRole('button',{name:'Delete state'})).toBeDisabled();
 await page.screenshot({path:info.outputPath('deleted-destination-excluded.png')});await select.selectOption(states[2].id);await page.getByRole('button',{name:'Delete state'}).click();await expect(page.getByRole('dialog')).toHaveCount(0);expect(writes).toBe(2);
});
test('failed targeted destination cannot be reselected without verification',async ({page})=>{
 await fixture(page);let writes=0;
 await page.route(`**/api/me/crm/states/${states[1].id}`,route=>route.request().method()==='GET'?route.fulfill({json:{...states[1],revision:2}}):(writes++,route.fulfill({status:409})));
 await page.route(`**/api/me/crm/states/${states[0].id}`,route=>route.fulfill({status:503}));
 await page.goto('/leads');await open(page);await page.getByRole('button',{name:'Delete state'}).click();await expect(page.getByRole('dialog')).toContainText('Destination state could not be refreshed');
 const select=page.getByRole('combobox',{name:'Move parcels to'});await select.selectOption(states[2].id);await select.selectOption(states[0].id);await expect(page.getByRole('dialog')).toContainText('Destination state could not be verified');await expect(page.getByRole('button',{name:'Delete state'})).toBeDisabled();await page.getByRole('dialog').locator('form').evaluate((form:HTMLFormElement)=>form.requestSubmit());expect(writes).toBe(1);
});

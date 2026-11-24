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
  let saves=0;
  await page.route(`**/api/me/favorites/${parcel}`,route=>{if(route.request().method()==='GET')return route.fulfill({status:404});saves++;return route.fulfill({json:{parcel_id:parcel,favorite:true,state_id:'11111111-1111-4111-8111-111111111111',revision:1,excluded:false,exclusion_reason:''}})});
  await page.goto(`/?parcel=${parcel}`);
  await expect(page.getByRole('heading',{name:'Parcel evidence'})).toBeVisible();
  await page.getByRole('button',{name:'Save favorite'}).click();
  await expect(page.getByRole('button',{name:'Saved to favorites'})).toBeVisible();
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
await new Promise<void>(resolve=>release=resolve);return route.fulfill({json:card(parcel)});
});
await page.route(`**/api/me/favorites/${b}`,route=>route.fulfill({json:card(b)}));
await page.route(`**/api/me/favorites/${b}/state`,route=>{expect(route.request().postDataJSON()).toEqual({state_id:newState,revision:3});state=newState;revision++;return route.fulfill({json:card(b)})});
await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:oldState,name:'New',position:0,revision:1,count:0},{id:newState,name:'Contacted',position:1,revision:1,count:1}],default_state_id:oldState,revision:1,next:''}}));
await page.route('**/api/me/crm/board/*',route=>route.fulfill({json:{items:route.request().url().includes(newState)&&state===newState?[{...card(b),parcel_number:'B-PARCEL',address:'B Street',favorited_at:'2026-11-24'}]:[],next:''}}));
await page.goto(`/?parcel=${parcel}`);await expect(page.getByRole('button',{name:'Save favorite'})).toBeVisible();await page.getByRole('button',{name:'Save favorite'}).click();
await page.getByRole('combobox',{name:'Find parcel ID or address'}).fill('B-PARCEL');await page.getByRole('option',{name:/B Street/}).click();
await expect(page.getByRole('combobox',{name:'Change saved parcel state'})).toBeVisible();release?.();await expect(page.getByRole('button',{name:'Saved to favorites'})).toBeVisible();
await page.getByRole('combobox',{name:'Change saved parcel state'}).selectOption(newState);await expect(page.getByText('State changed.')).toBeVisible();await page.screenshot({path:info.outputPath('04-discovery-state.png'),fullPage:true});
await page.goto(`/?parcel=${b}`);await expect(page.getByRole('combobox',{name:'Change saved parcel state'})).toHaveValue(newState);
await page.goto('/favorites');await expect(page.getByRole('region',{name:'Contacted'}).getByText('B-PARCEL')).toBeVisible();await page.screenshot({path:info.outputPath('05-board-state.png'),fullPage:true});
});

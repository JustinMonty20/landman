import { expect, test } from '@playwright/test';
const origin = 'http://127.0.0.1:5174';
const first = '11111111-1111-4111-8111-111111111111';
const second = '22222222-2222-4222-8222-222222222222';
const parcel = '33333333-3333-4333-8333-333333333333';
test.use({ video: 'on' });
test('private favorites board supports keyboard state move and reload', async ({ page }, info) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  await page.route('**/api/me', route => route.fulfill({ json: { id: 'buyer-fixture', email: 'buyer@example.test', name: 'Buyer' } }));
  let current = first; let revision = 1;
  const states = [{ id: first, name: 'New', position: 0, revision: 1, count: 1 }, { id: second, name: 'Contacted', position: 1, revision: 1, count: 0 }];
  await page.route('**/api/me/crm/**', route => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/me/crm/board') {
      return route.fulfill({ json: { states: states.map(s => ({ ...s, count: current === s.id ? 1 : 0 })), default_state_id: first, revision: 1, next: "" } });
    }
    if (url.pathname.startsWith('/api/me/crm/board/')) {
      const state = url.pathname.split('/').at(-1);
      return route.fulfill({ json: { items: current === state ? [{ parcel_id: parcel, parcel_number: '009845', address: '10 Fixture Lane', state_id: current, favorite: true, excluded: false, exclusion_reason: '', revision, favorited_at: '2026-11-24 12:00:00+00' }] : [], next: '' } });
    }
    return route.abort();
  });
  await page.route(`**/api/me/favorites/${parcel}/state`, async route => {
    const body = route.request().postDataJSON();
    expect(body).toEqual({ state_id: second, revision: 1 });
    current = second; revision++;
    return route.fulfill({ json: { parcel_id: parcel, state_id: current, revision } });
  });
  await page.goto('/favorites');
  await expect(page.getByRole('heading', { name: 'Favorites board' })).toBeVisible();
  await expect(page.getByText('009845')).toBeVisible();
  await page.screenshot({ path: info.outputPath('01-new.png'), fullPage: true });
  const selector = page.getByRole('combobox', { name: 'Change state for 009845' });
  await selector.focus();
  await selector.press('ArrowDown');
  await selector.press('Enter');
  await expect(page.getByRole('region', { name: 'Contacted' }).getByText('009845')).toBeVisible();
  await page.reload();
  await expect(page.getByRole('region', { name: 'Contacted' }).getByText('009845')).toBeVisible();
  await page.screenshot({ path: info.outputPath('02-contacted.png'), fullPage: true });
});
test('column pagination ignores duplicate clicks and stale filter pages', async ({page}) => {
await page.route('**/*', route => {const url=new URL(route.request().url());if(url.origin!==origin || /^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:first,name:'New',position:0,revision:1,count:2}],default_state_id:first,revision:1,next:''}}));
let resolvePage: (()=>void)|undefined;let requested=0;
await page.route(`**/api/me/crm/board/${first}*`,async route=>{
const url=new URL(route.request().url());
if(url.searchParams.has('cursor')) {requested++;await new Promise<void>(resolve=>resolvePage=resolve);return route.fulfill({json:{items:[{parcel_id:second,parcel_number:'SECOND',address:null,state_id:first,revision:1,excluded:false,exclusion_reason:'',favorited_at:'2026-11-25'}],next:''}})}
return route.fulfill({json:{items:[{parcel_id:parcel,parcel_number:'FIRST',address:null,state_id:first,revision:1,excluded:false,exclusion_reason:'',favorited_at:'2026-11-24'}],next:url.searchParams.get('hide_excluded')==='true'?'':'cursor'}})
});
await page.goto('/favorites');
const more=page.getByRole('button',{name:'Load more in New'});await expect(more).toBeVisible();await more.dblclick({delay:5});await expect(more).toBeDisabled();expect(requested).toBe(1);
await page.getByRole('checkbox',{name:'Hide excluded'}).check();await expect(more).toHaveCount(0);resolvePage?.();await expect(page.getByText('SECOND')).toHaveCount(0);await expect(page.getByText('FIRST')).toHaveCount(1);
});
test('old page rejection cannot overwrite reloaded board; retry clears current error', async ({page}) => {
await page.route('**/*',route=>{const url=new URL(route.request().url());if(url.origin!==origin || /^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:first,name:'New',position:0,revision:1,count:1}],default_state_id:first,revision:1,next:''}}));
let release:(()=>void)|undefined, pages=0;
await page.route(`**/api/me/crm/board/${first}*`,async route=>{
const url=new URL(route.request().url());
if(url.searchParams.has('cursor')){pages++;if(pages===1)await new Promise<void>(resolve=>release=resolve);return route.fulfill({status:503,body:'unavailable'})}
return route.fulfill({json:{items:[{parcel_id:parcel,parcel_number:'FIRST',address:null,state_id:first,revision:1,excluded:false,exclusion_reason:'',favorited_at:'2026-11-24'}],next:'cursor'}});
});
await page.goto('/favorites');const more=page.getByRole('button',{name:'Load more in New'});await expect(more).toBeVisible();await more.click();
await page.getByRole('button',{name:'Reload board'}).click();await expect(more).toBeEnabled();
const staleResponse=page.waitForResponse(response=>response.url().includes('/api/me/crm/board/') && response.status()===503);release?.();await staleResponse;
await expect(page.getByRole('alert')).toHaveCount(0);
await more.click();await expect(page.getByRole('alert')).toContainText('503');
await page.route(`**/api/me/crm/board/${first}*`,route=>route.fulfill({json:{items:[{parcel_id:second,parcel_number:'SECOND',address:null,state_id:first,revision:1,excluded:false,exclusion_reason:'',favorited_at:'2026-11-25'}],next:''}}));
await more.click();await expect(page.getByText('SECOND')).toBeVisible();await expect(page.getByRole('alert')).toHaveCount(0);
});
test('column failure stays scoped while another column succeeds and retry recovers',async ({page},info)=>{
await page.emulateMedia({reducedMotion:'reduce'});
await page.route('**/*',route=>{const u=new URL(route.request().url());if(u.origin!==origin||/^\/(api|auth)(\/|$)/.test(u.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:first,name:'New',count:1,position:0,revision:1},{id:second,name:'Contacted',count:1,position:1,revision:1}],default_state_id:first,revision:1,next:''}}));
let fail=true;
await page.route('**/api/me/crm/board/*',route=>{const id=new URL(route.request().url()).pathname.split('/').at(-1);if(id===first&&fail)return route.fulfill({status:503});return route.fulfill({json:{items:[{parcel_id:id===first?first:second,parcel_number:id===first?'A-CARD':'B-CARD',address:null,state_id:id,revision:1,excluded:false,exclusion_reason:'',favorited_at:'2026-01-01'}],next:''}})});
await page.goto('/favorites');const a=page.getByRole('region',{name:'New'}),b=page.getByRole('region',{name:'Contacted'});
await expect(a.getByRole('alert')).toContainText('503');await expect(a.getByText('No favorites in this state.')).toHaveCount(0);await expect(b.getByText('B-CARD')).toBeVisible();await expect(a.getByRole('alert')).toContainText('503');
await page.screenshot({path:info.outputPath('column-error.png'),fullPage:true});fail=false;await a.getByRole('button',{name:'Retry New cards'}).click();await expect(a.getByText('A-CARD')).toBeVisible();await expect(a.getByRole('alert')).toHaveCount(0);
});
test('state-page error retries independently and stale failure cannot overwrite new filter',async ({page},info)=>{
await page.emulateMedia({reducedMotion:'reduce'});
await page.route('**/*',route=>{const u=new URL(route.request().url());if(u.origin!==origin||/^\/(api|auth)(\/|$)/.test(u.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
let fail=true,release:(()=>void)|undefined,stale=false;
await page.route('**/api/me/crm/board*',async route=>{const cursor=new URL(route.request().url()).searchParams.get('cursor');if(cursor&&stale){await new Promise<void>(resolve=>release=resolve);return route.fulfill({status:503})}if(cursor&&fail)return route.fulfill({status:503});return route.fulfill({json:{states:cursor?[{id:second,name:'Contacted',count:0,position:1,revision:1}]:[{id:first,name:'New',count:0,position:0,revision:1}],default_state_id:first,revision:1,next:cursor?'':'cursor'}})});
await page.route('**/api/me/crm/board/*',route=>route.fulfill({json:{items:[],next:''}}));
await page.goto('/favorites');const more=page.getByRole('button',{name:'Load more states'});await more.click();await expect(page.getByRole('alert')).toContainText('503');await page.screenshot({path:info.outputPath('state-page-error.png'),fullPage:true});fail=false;await more.click();await expect(page.getByRole('region',{name:'Contacted'})).toBeVisible();await expect(page.getByRole('alert')).toHaveCount(0);
await page.getByRole('button',{name:'Reload board'}).click();stale=true;await more.click();await page.getByRole('checkbox',{name:'Hide excluded'}).check();await expect(page.getByRole('region',{name:'New'})).toBeVisible();release?.();await expect(page.getByRole('alert')).toHaveCount(0);
});
test('board pages beyond fifty states preserve a selected card and expose state 51',async ({page},info)=>{
await page.emulateMedia({reducedMotion:'reduce'});
await page.route('**/*',route=>{const u=new URL(route.request().url());if(u.origin!==origin||/^\/(api|auth)(\/|$)/.test(u.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
const states=Array.from({length:51},(_,i)=>({id:`${String(i+1).padStart(8,'0')}-1111-4111-8111-111111111111`,name:`State ${i+1}`,position:i,revision:1,count:i===0||i===50?1:0}));
await page.route('**/api/me/crm/board*',route=>{const cursor=new URL(route.request().url()).searchParams.get('cursor');return route.fulfill({json:{states:cursor?states.slice(50):states.slice(0,50),next:cursor?'':'cursor',default_state_id:states[0].id,revision:1}})});
await page.route('**/api/me/crm/board/*',route=>{const id=new URL(route.request().url()).pathname.split('/').at(-1);return route.fulfill({json:{items:id===states[0].id||id===states[50].id?[{parcel_id:id,parcel_number:id===states[0].id?'FIRST':'FIFTY-FIRST',address:null,state_id:id,revision:1,excluded:false,exclusion_reason:'',favorited_at:'2026-01-01'}]:[],next:''}})});
await page.goto('/favorites');const firstCard=page.getByRole('region',{name:'State 1'}).getByRole('combobox',{name:'Change state for FIRST'});await expect(firstCard).toHaveValue(states[0].id);await expect(firstCard.locator('option:checked')).toHaveText('State 1');
await page.getByRole('button',{name:'Load more states'}).click();const last=page.getByRole('region',{name:'State 51'});await expect(last.getByText('FIFTY-FIRST')).toBeVisible();await expect(last.getByRole('combobox',{name:'Change state for FIFTY-FIRST'}).locator('option:checked')).toHaveText('State 51');await expect(firstCard).toHaveValue(states[0].id);await expect(firstCard.locator('option')).toHaveCount(51);
await last.scrollIntoViewIfNeeded();await page.screenshot({path:info.outputPath('board-state-51.png'),fullPage:true});
});

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

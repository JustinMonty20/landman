import { expect, test } from '@playwright/test';
const origin='http://127.0.0.1:5174';
const a='11111111-1111-4111-8111-111111111111';
const b='22222222-2222-4222-8222-222222222222';
type Note={id:string;user_id:string;parcel_id:string;body:string;created_at:string;updated_at:string;revision:number};
const note=(id:string,parcel_id:string,body:string,revision=1):Note=>({id,user_id:'fixture',parcel_id,body,revision,created_at:'2026-11-01T12:00:00Z',updated_at:revision===1?'2026-11-01T12:00:00Z':'2026-11-02T12:00:00Z'});
async function fixture(page:import('@playwright/test').Page){
 await page.route('**/*',route=>{const url=new URL(route.request().url());if(url.origin!==origin||/^\/(api|auth)(\/|$)/.test(url.pathname))return route.abort();return route.continue()});
 await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
 await page.route('**/api/coverage*',route=>route.fulfill({json:{counties:[]}}));
 await page.route('**/api/me/favorites/*',route=>route.fulfill({status:404}));
}
test('unsaved parcel scratchpad creates, edits, pages and confirms deletion',async ({page},info)=>{
 await fixture(page);const records:Note[]=[];
 await page.route(/\/api\/me\/favorites\/[^/]+\/notes(?:\/[^/?]+)?(?:\?.*)?$/, route=>{
  const req=route.request(),url=new URL(req.url()),id=url.pathname.split('/').at(-1)!;
  if(req.method()==='GET')return route.fulfill({json:{items:url.searchParams.has('cursor')?records.slice(1):records.slice(0,1),next:records.length>1&&!url.searchParams.has('cursor')?'cursor':''}});
  if(req.method()==='POST'){const data=req.postDataJSON();const found=records.find(n=>n.id===data.id);if(found)return route.fulfill({json:found});const created=note(data.id,a,data.body);records.unshift(created);return route.fulfill({json:created})}
  if(req.method()==='PUT'){const data=req.postDataJSON(),index=records.findIndex(n=>n.id===id);records[index]=note(id,a,data.body,2);return route.fulfill({json:records[index]})}
  if(req.method()==='DELETE'){records.splice(records.findIndex(n=>n.id===id),1);return route.fulfill({status:204})}
  return route.abort();
 });
 await page.goto(`/?parcel=${a}`);
 const scratch=page.getByRole('region',{name:'Private parcel scratchpad'});
 await expect(scratch.getByText('No notes yet.')).toBeVisible();
 await scratch.getByRole('textbox',{name:'New note'}).fill('<script>alert(1)</script>\nFixture note');
 await scratch.getByRole('button',{name:'Add note'}).click();
 await expect(scratch.getByText('<script>alert(1)</script>\nFixture note')).toBeVisible();
 expect(await scratch.locator('script').count()).toBe(0);
 await scratch.screenshot({path:info.outputPath('01-created.png')});
 await scratch.getByRole('button',{name:'Edit'}).click();await scratch.getByRole('textbox',{name:'Edit note'}).fill('Edited fixture note');
 await scratch.getByRole('button',{name:'Save note'}).click();
 await expect(scratch.getByText('Edited fixture note')).toBeVisible();await expect(scratch.getByText('Edited', {exact:false}).first()).toBeVisible();
 records.push(note('33333333-3333-4333-8333-333333333333',a,'Older fixture note'));
 await page.reload();await expect(scratch.getByRole('button',{name:'Load older notes'})).toBeVisible();await scratch.getByRole('button',{name:'Load older notes'}).click();await expect(scratch.getByText('Older fixture note')).toBeVisible();
 await scratch.getByRole('button',{name:'Delete…'}).first().click();
 await expect(page.getByRole('dialog')).toContainText('Delete note?');await page.screenshot({path:info.outputPath('02-delete-confirmation.png')});await page.getByRole('dialog').getByRole('button',{name:'Cancel'}).click();
 await expect(scratch.getByText('Edited fixture note')).toBeVisible();
 await scratch.getByRole('button',{name:'Delete…'}).first().click();await page.getByRole('dialog').getByRole('button',{name:'Delete note'}).click();
 await expect(scratch.getByText('Edited fixture note')).toHaveCount(0);
 await scratch.screenshot({path:info.outputPath('03-after-delete.png')});
});
test('switching parcels preserves drafts and blocks stale notes and stale writes',async ({page})=>{
 await fixture(page);let release:(()=>void)|undefined;
 await page.route(/\/api\/me\/favorites\/[^/]+\/notes(?:\/[^/?]+)?(?:\?.*)?$/, async route=>{
  const url=new URL(route.request().url());if(url.pathname.includes(a)&&route.request().method()==='GET')await new Promise<void>(resolve=>release=resolve);
  return route.fulfill({json:{items:[note('44444444-4444-4444-8444-444444444444',url.pathname.includes(a)?a:b,url.pathname.includes(a)?'A secret':'B entry')],next:''}});
 });
 await page.route('**/api/v1/parcels/lookup*',route=>route.fulfill({json:{items:[{id:b,parcel_number:'BPARCEL',address:'Switch B'},{id:a,parcel_number:'APARCEL',address:'Switch A'}]}}));
 await page.goto(`/?parcel=${a}`);await page.getByRole('textbox',{name:'New note'}).fill('Draft for A');
 const finder=page.getByRole('combobox',{name:'Find parcel ID or address'});
 await finder.fill('Switch');await page.getByRole('option',{name:'Switch B · BPARCEL'}).click();
 const scratch=page.getByRole('region',{name:'Private parcel scratchpad'});
 await expect(scratch.getByText('B entry')).toBeVisible();release?.();await expect(scratch.getByText('A secret')).toHaveCount(0);
 await finder.fill('Switch');await page.getByRole('option',{name:'Switch A · APARCEL'}).click();
 await expect(scratch.getByRole('textbox',{name:'New note'})).toHaveValue('Draft for A');
});
test('a pending note load cannot replace a newly submitted note',async ({page})=>{
 await fixture(page);let release:(()=>void)|undefined;
 await page.route(/\/api\/me\/favorites\/[^/]+\/notes(?:\?.*)?$/,async route=>{
  if(route.request().method()==='GET')await new Promise<void>(resolve=>release=resolve);
  return route.fulfill({json:{items:[],next:''}});
 });
 await page.goto(`/?parcel=${a}`);
 const scratch=page.getByRole('region',{name:'Private parcel scratchpad'});
 await expect(scratch.getByRole('status')).toContainText('Loading notes');
 await scratch.getByRole('textbox',{name:'New note'}).fill('Keep draft while loading');
 await expect(scratch.getByRole('button',{name:'Add note'})).toBeDisabled();
 release?.();
 await expect(scratch.getByRole('button',{name:'Add note'})).toBeEnabled();
 await expect(scratch.getByRole('textbox',{name:'New note'})).toHaveValue('Keep draft while loading');
});
test('failed create retries same ID; conflict preserves edit text',async ({page})=>{
 await fixture(page);let attempts:string[]=[],current=note('55555555-5555-4555-8555-555555555555',a,'Original');let created=false;
 await page.route(/\/api\/me\/favorites\/[^/]+\/notes(?:\/[^/?]+)?(?:\?.*)?$/, route=>{
  const req=route.request();if(req.method()==='GET')return route.fulfill({json:{items:created?[current]:[],next:''}});
  if(req.method()==='POST'){const data=req.postDataJSON();attempts.push(data.id);if(attempts.length===1)return route.fulfill({status:503});created=true;current=note(data.id,a,data.body);return route.fulfill({json:current})}
  if(req.method()==='PUT')return route.fulfill({status:409});return route.abort();
 });
 await page.goto(`/?parcel=${a}`);const scratch=page.getByRole('region',{name:'Private parcel scratchpad'});
 await scratch.getByRole('textbox',{name:'New note'}).fill('Keep this');await scratch.getByRole('button',{name:'Add note'}).click();
 await expect(scratch.getByRole('alert')).toContainText('text is preserved');await expect(scratch.getByRole('textbox',{name:'New note'})).toHaveValue('Keep this');
 await scratch.getByRole('button',{name:'Add note'}).click();expect(attempts[0]).toBe(attempts[1]);
 await scratch.getByRole('button',{name:'Edit'}).click();await scratch.getByRole('textbox',{name:'Edit note'}).fill('Unsent edit');await scratch.getByRole('button',{name:'Save note'}).click();
 await expect(scratch.getByRole('alert')).toContainText('changed in another tab');await expect(scratch.getByRole('textbox',{name:'Edit note'})).toHaveValue('Unsent edit');
 await scratch.getByRole('button',{name:'Reload notes'}).click();await expect(scratch.getByRole('textbox',{name:'Edit note'})).toHaveValue('Unsent edit');
});

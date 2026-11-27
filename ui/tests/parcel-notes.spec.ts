import { expect, test } from '@playwright/test';
// Show native scrollbars in parcel screenshots; Playwright hides them by default.
test.use({ launchOptions: { args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'], ignoreDefaultArgs: ['--hide-scrollbars'] } });
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
test('bounded preview and chronological timeline, inline older-note deletion and keyboard controls',async ({page},info)=>{
 await fixture(page);
 const long='Long opening '+ 'x'.repeat(3900);
 const records=[note('33333333-3333-4333-8333-333333333333',a,'Newest'),note('22222222-2222-4222-8222-222222222222',a,'Middle'),note('11111111-1111-4111-8111-111111111111',a,long),note('00000000-0000-4000-8000-000000000000',a,'Oldest')];
 const deleted:string[]=[];const requests:string[]=[];
 await page.route(/\/api\/me\/favorites\/[^/]+\/notes(?:\/[^/?]+)?(?:\?.*)?$/, route=>{
  const req=route.request(),url=new URL(req.url()),id=url.pathname.split('/').at(-1)!;
  if(req.method()==='GET'){
   const asc=url.searchParams.get('order')==='asc',ordered=asc?[...records].reverse():records;
   const start=url.searchParams.has('cursor')?Number(url.searchParams.get('cursor')):0,limit=asc?2:3;
   requests.push(`${url.searchParams.get('order')??'desc'}:${start}`);
   return route.fulfill({json:{items:ordered.slice(start,start+limit),next:start+limit<ordered.length?String(start+limit):''}});
  }
  if(req.method()==='DELETE'){deleted.push(id);records.splice(records.findIndex(n=>n.id===id),1);return route.fulfill({status:204})}
  if(req.method()==='POST'){const data=req.postDataJSON();const found=records.find(n=>n.id===data.id);if(found)return route.fulfill({json:found});const created=note(data.id,a,data.body);records.unshift(created);return route.fulfill({json:created})}
  if(req.method()==='PUT'){const data=req.postDataJSON(),index=records.findIndex(n=>n.id===id);records[index]=note(id,a,data.body,2);return route.fulfill({json:records[index]})}
  return route.abort();
 });
 await page.goto(`/?parcel=${a}`);await page.getByRole('tab',{name:'Notes'}).click();
 const scratch=page.getByRole('region',{name:'Private parcel scratchpad'});
 await expect(scratch.getByRole('heading',{name:'Scratchpad',exact:true})).toBeVisible();
 await expect(scratch.getByRole('textbox',{name:'Note text'})).not.toHaveAttribute('placeholder',/New note/i);
 await expect(scratch.getByRole('listitem')).toHaveCount(3);
 await expect(scratch.getByText(long)).toHaveCount(0);
 await expect(scratch.getByText('Long opening', {exact:false})).toBeVisible();
 await expect(scratch.getByText(/UTF-8|bytes/)).toHaveCount(0);
 await expect(page.getByRole('dialog')).toHaveCount(0);
 await scratch.scrollIntoViewIfNeeded();
 await page.screenshot({path:info.outputPath('00-parcel-panel.png')});
 await scratch.screenshot({path:info.outputPath('01-compact-panel.png')});
 await scratch.getByRole('textbox',{name:'Note text'}).fill('Draft survives navigation');
 await scratch.getByRole('link',{name:'Notes'}).click();
 await expect(page).toHaveURL(new RegExp(`/notes\\?parcel=${a}`));
 await expect(page.getByText(`Parcel ${a}`)).toBeVisible();
 const timeline=page.getByRole('region',{name:'Private parcel scratchpad'});
 await expect(timeline.getByRole('textbox',{name:'Note text'})).toHaveValue('Draft survives navigation');
 await expect(timeline.getByRole('listitem')).toHaveCount(2);
 await expect(timeline.getByRole('listitem').first()).toContainText('Oldest');
 await timeline.getByRole('button',{name:'Load more notes'}).click();
 await expect(timeline.getByRole('listitem')).toHaveCount(4);
 await expect(timeline.getByRole('listitem').nth(1)).toContainText(long);
 await expect(timeline.getByRole('listitem').last()).toContainText('Newest');
 expect(requests).toContain('asc:2');
 await page.screenshot({path:info.outputPath('03-full-timeline.png'),fullPage:true});
 const older=timeline.getByRole('listitem').nth(1);
 await older.getByRole('button',{name:'Delete note',exact:true}).focus();
 await expect(older.getByRole('button',{name:'Delete note',exact:true})).toBeFocused();
 await page.keyboard.press('Enter');
 await expect(older).toContainText('Delete this note?');
 await expect(page.getByRole('dialog')).toHaveCount(0);
 await page.screenshot({path:info.outputPath('02-inline-confirmation.png'),fullPage:true});
 await older.getByRole('button',{name:'Cancel delete note'}).click();
 await expect(older).toContainText(long);
 expect(deleted).toEqual([]);
 await older.getByRole('button',{name:'Delete note',exact:true}).click();
 await older.getByRole('button',{name:'Confirm delete note'}).click();
 expect(deleted).toEqual(['11111111-1111-4111-8111-111111111111']);
 await expect(timeline.getByRole('listitem')).toHaveCount(3);
 const oldest=timeline.getByRole('listitem').first();
 await oldest.getByRole('button',{name:'Edit note'}).click();
 await oldest.getByRole('textbox',{name:'Edit note'}).fill('Keep original on cancel');
 await oldest.getByRole('button',{name:'Cancel edit'}).click();
 await expect(oldest).toContainText('Oldest');
 await oldest.getByRole('button',{name:'Edit note'}).click();
 await oldest.getByRole('textbox',{name:'Edit note'}).fill('Updated oldest');
 await oldest.getByRole('button',{name:'Save note'}).click();
 await expect(oldest).toContainText('Updated oldest');
 await timeline.getByRole('textbox',{name:'Note text'}).fill('a'.repeat(4001));
 await expect(timeline.getByText('Note is too long. Shorten it before saving.')).toBeVisible();
 await expect(timeline.getByRole('button',{name:'Add note'})).toBeDisabled();
 await timeline.getByRole('textbox',{name:'Note text'}).fill('Draft survives navigation');
 await page.getByRole('link',{name:'Return to parcel'}).click();await page.getByRole('tab',{name:'Notes'}).click();
 await expect(page).toHaveURL(new RegExp(`parcel=${a}`));
 await expect(page.getByRole('textbox',{name:'Note text'})).toHaveValue('Draft survives navigation');
 await scratch.getByRole('textbox',{name:'Note text'}).fill('<script>alert(1)</script>');
 await scratch.getByRole('button',{name:'Add note'}).click();
 await expect(scratch.getByText('<script>alert(1)</script>')).toBeVisible();
 expect(await scratch.locator('script').count()).toBe(0);
});
test('switching parcels preserves drafts and blocks stale notes and stale writes',async ({page})=>{
 await fixture(page);let release:(()=>void)|undefined;
 await page.route(/\/api\/me\/favorites\/[^/]+\/notes(?:\/[^/?]+)?(?:\?.*)?$/, async route=>{
  const url=new URL(route.request().url());if(url.pathname.includes(a)&&route.request().method()==='GET')await new Promise<void>(resolve=>release=resolve);
  return route.fulfill({json:{items:[note('44444444-4444-4444-8444-444444444444',url.pathname.includes(a)?a:b,url.pathname.includes(a)?'A secret':'B entry')],next:''}});
 });
 await page.route('**/api/v1/parcels/lookup*',route=>route.fulfill({json:{items:[{id:b,parcel_number:'BPARCEL',address:'Switch B'},{id:a,parcel_number:'APARCEL',address:'Switch A'}]}}));
 await page.goto(`/?parcel=${a}`);await page.getByRole('tab',{name:'Notes'}).click();await page.getByRole('textbox',{name:'Note text'}).fill('Draft for A');
 const finder=page.getByRole('combobox',{name:'Find parcel ID or address'});
 await finder.fill('Switch');await page.getByRole('option',{name:'Switch B · BPARCEL'}).click();
 const scratch=page.getByRole('region',{name:'Private parcel scratchpad'});
 await expect(scratch.getByText('B entry')).toBeVisible();release?.();await expect(scratch.getByText('A secret')).toHaveCount(0);
 await finder.fill('Switch');await page.getByRole('option',{name:'Switch A · APARCEL'}).click();
 await expect(scratch.getByRole('textbox',{name:'Note text'})).toHaveValue('Draft for A');
});
test('a pending note load cannot replace a newly submitted note',async ({page},info)=>{
 await fixture(page);let release:(()=>void)|undefined;
 await page.route(/\/api\/me\/favorites\/[^/]+\/notes(?:\?.*)?$/,async route=>{
  if(route.request().method()==='GET')await new Promise<void>(resolve=>release=resolve);
  return route.fulfill({json:{items:[],next:''}});
 });
 await page.goto(`/?parcel=${a}`);await page.getByRole('tab',{name:'Notes'}).click();
 const scratch=page.getByRole('region',{name:'Private parcel scratchpad'});
 await expect(scratch.getByRole('status')).toContainText('Loading notes');
 await scratch.getByRole('textbox',{name:'Note text'}).fill('Keep draft while loading');
 await expect(scratch.getByRole('button',{name:'Add note'})).toBeDisabled();
 release?.();
 await expect(scratch.getByRole('button',{name:'Add note'})).toBeEnabled();
 await expect(scratch.getByRole('textbox',{name:'Note text'})).toHaveValue('Keep draft while loading');
 await expect(scratch.getByText('No notes yet.',{exact:true})).toHaveCount(0);
 await expect(scratch.getByText('New note',{exact:true})).toHaveCount(0);
 const textbox=await scratch.getByRole('textbox',{name:'Note text'}).boundingBox();
 const add=await scratch.getByRole('button',{name:'Add note'}).boundingBox();
 expect(textbox).not.toBeNull();expect(add).not.toBeNull();
 expect(add!.y-(textbox!.y+textbox!.height)).toBeGreaterThanOrEqual(12);
 await scratch.screenshot({path:info.outputPath('04-quiet-composer.png')});
});
test('failed create retries same ID; conflict preserves edit text',async ({page})=>{
 await fixture(page);let attempts:string[]=[],current=note('55555555-5555-4555-8555-555555555555',a,'Original');let created=false;
 await page.route(/\/api\/me\/favorites\/[^/]+\/notes(?:\/[^/?]+)?(?:\?.*)?$/, route=>{
  const req=route.request();if(req.method()==='GET')return route.fulfill({json:{items:created?[current]:[],next:''}});
  if(req.method()==='POST'){const data=req.postDataJSON();attempts.push(data.id);if(attempts.length===1)return route.fulfill({status:503});created=true;current=note(data.id,a,data.body);return route.fulfill({json:current})}
  if(req.method()==='PUT'||req.method()==='DELETE')return route.fulfill({status:409});return route.abort();
 });
 await page.goto(`/?parcel=${a}`);await page.getByRole('tab',{name:'Notes'}).click();const scratch=page.getByRole('region',{name:'Private parcel scratchpad'});
 await scratch.getByRole('textbox',{name:'Note text'}).fill('Keep this');await scratch.getByRole('button',{name:'Add note'}).click();
 await expect(scratch.getByRole('alert')).toContainText('text is preserved');await expect(scratch.getByRole('textbox',{name:'Note text'})).toHaveValue('Keep this');
 await page.getByRole('tab',{name:'Research'}).click();await page.getByRole('tab',{name:'Notes'}).click();
 await expect(scratch.getByRole('textbox',{name:'Note text'})).toHaveValue('Keep this');
 await scratch.getByRole('button',{name:'Add note'}).click();expect(attempts[0]).toBe(attempts[1]);
 await scratch.getByRole('button',{name:'Edit note'}).click();await scratch.getByRole('textbox',{name:'Edit note'}).fill('Unsent edit');await scratch.getByRole('button',{name:'Save note'}).click();
 await expect(scratch.getByRole('alert')).toContainText('changed in another tab');await expect(scratch.getByRole('textbox',{name:'Edit note'})).toHaveValue('Unsent edit');
 await page.getByRole('tab',{name:'Evidence',exact:true}).click();await page.getByRole('tab',{name:'Notes'}).click();
 await expect(scratch.getByRole('textbox',{name:'Edit note'})).toHaveValue('Unsent edit');
 await scratch.getByRole('button',{name:'Reload notes'}).click();await expect(scratch.getByRole('textbox',{name:'Edit note'})).toHaveValue('Unsent edit');
 await scratch.getByRole('button',{name:'Cancel edit'}).click();
 await scratch.getByRole('button',{name:'Delete note',exact:true}).click();
 await scratch.getByRole('button',{name:'Confirm delete note'}).click();
 await expect(scratch.getByRole('alert')).toContainText('changed in another tab');
 await expect(scratch.getByText('Keep this')).toBeVisible();
 await expect(scratch.getByRole('button',{name:'Confirm delete note'})).toBeDisabled();
});
test('full-size parcel tabs scroll horizontally while notes and tasks retain drafts and bounded content',async ({page},info)=>{
 await fixture(page);
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.setViewportSize({width:info.project.name==='tablet'?768:1440,height:560});
 const parcelNumber=('00012345 ').repeat(100);
 await page.route(`**/api/v1/parcels/${a}`,route=>route.fulfill({json:{location:{type:'Point',coordinates:[-80.7,34.89]},parcel:{parcel_number:parcelNumber,address:'Fixture Way',mapped_acres:'2.5',updated_at:'2026-01-01'},source:'fixture',source_parcel_id:'fixture',owner_labels:null,sales:[]}}));
 await page.route(/\/api\/me\/favorites\/[^/]+\/notes(?:\?.*)?$/,route=>route.fulfill({json:{items:[note('33333333-3333-4333-8333-333333333333',a,'A'.repeat(130)),note('44444444-4444-4444-8444-444444444444',a,'B'.repeat(130)),note('55555555-5555-4555-8555-555555555555',a,'C'.repeat(130))],next:''}}));
 let saved=false;
 await page.route(`**/api/me/favorites/${a}`,route=>{
  if(route.request().method()==='PUT'){saved=true;return route.fulfill({json:{parcel_id:a,revision:1,favorite:true,excluded:false,exclusion_reason:'',state_id:'fixture'}})}
  return route.fulfill(saved?{json:{parcel_id:a,revision:1,favorite:true,excluded:false,exclusion_reason:'',state_id:'fixture'}}:{status:404});
 });
 await page.route(`**/api/me/favorites/${a}/tasks*`,route=>route.fulfill({json:{items:[],next:''}}));
 await page.goto(`/?parcel=${a}`);
 const panel=page.getByRole('region',{name:'Parcel evidence'});
 const tabs=panel.getByRole('tablist',{name:'Parcel evidence views'});
 await expect(tabs.getByRole('tab')).toHaveText(['Evidence','Research','Notes','Tasks']);
 const size=await tabs.evaluate(el=>{
  const button=el.querySelector('button')!;
  const style=getComputedStyle(button);
  return {viewport:el.clientWidth,content:el.scrollWidth,button:button.getBoundingClientRect().width,
   font:style.fontSize,padding:style.paddingInline,height:button.getBoundingClientRect().height,
   overflow:getComputedStyle(el).overflowX,scrollbar:getComputedStyle(el).scrollbarWidth,
   scrollbarPixels:el.offsetHeight-el.clientHeight,scrollLeft:el.scrollLeft};
 });
 const original=await page.getByRole('tablist',{name:'Map controls'}).getByRole('tab').first().evaluate(el=>{
  const style=getComputedStyle(el);
  return {font:style.fontSize,padding:style.paddingInline,height:el.getBoundingClientRect().height};
 });
 expect(size.content).toBeGreaterThan(size.viewport);
 expect(size.button).toBeGreaterThanOrEqual(size.viewport/2-1);
 expect(size.button).toBeLessThanOrEqual(size.viewport/2+1);
 expect(size.scrollLeft).toBe(0);
 expect(size.overflow).toBe('auto');
 expect(size.scrollbar).toBe('thin');
 expect(size.scrollbarPixels).toBeGreaterThan(2);
 expect({font:size.font,padding:size.padding,height:size.height}).toEqual(original);
 async function expectTabRevealed(name:string){
  const tab=tabs.getByRole('tab',{name,exact:true});
  await expect(tab).toBeFocused();
  await expect(tab).toHaveAttribute('data-selected','');
  await expect.poll(()=>tabs.evaluate((list,label)=>{
   const bounds=list.getBoundingClientRect();
   const button=Array.from(list.querySelectorAll('button')).find(el=>el.textContent===label)!.getBoundingClientRect();
   return button.left>=bounds.left-1 && button.right<=bounds.right+1;
  },name)).toBe(true);
 }
 await page.screenshot({path:info.outputPath('four-tabs-evidence.png')});
 await tabs.evaluate(el=>el.scrollLeft=el.scrollWidth);
 expect(await tabs.evaluate(el=>el.scrollLeft)).toBeGreaterThan(0);
 await expect(tabs.getByRole('tab',{name:'Tasks'})).toBeInViewport();
 await tabs.getByRole('tab',{name:'Tasks'}).click();
 await expect(tabs.getByRole('tab',{name:'Tasks'})).toHaveAttribute('data-selected','');
 await tabs.getByRole('tab',{name:'Evidence'}).click();
 await expect(tabs.getByRole('tab',{name:'Evidence'})).toHaveAttribute('data-selected','');
 await expect(panel.getByRole('heading',{name:'Parcel evidence'})).toBeVisible();
 await expect(panel.getByRole('button',{name:'Close parcel details'})).toBeVisible();
 await expect(panel.getByRole('button',{name:'Favorite parcel'})).toBeVisible();
 await expect(panel.getByRole('region',{name:'Private parcel scratchpad'})).toBeHidden();
 await expect(panel.getByRole('region',{name:'Parcel follow-up tasks'})).toHaveCount(0);
 const heading=panel.getByRole('heading',{name:'Parcel evidence'});
 const fixedY=(await heading.boundingBox())!.y;
 const tabY=(await tabs.boundingBox())!.y;
 async function scrollTab(name:string,path:string){
  const tab=panel.getByRole('tab',{name,exact:true});
  const content=panel.getByRole('tabpanel',{name,exact:true});
  await expect(tab).toHaveAttribute('data-selected','');
  await expect.poll(()=>content.evaluate(el=>el.scrollHeight>el.clientHeight)).toBe(true);
  await content.evaluate(el=>el.scrollTop=el.scrollHeight);
  await expect.poll(()=>content.evaluate(el=>el.scrollTop)).toBeGreaterThan(0);
  expect((await heading.boundingBox())!.y).toBe(fixedY);
  expect((await tabs.boundingBox())!.y).toBe(tabY);
  expect(await panel.evaluate(el=>el.scrollTop)).toBe(0);
  await page.screenshot({path:info.outputPath(`${path}-scrolled.png`)});
 }
 await expect(panel.getByRole('region',{name:'Land and value'})).toBeVisible();
 await scrollTab('Evidence','evidence');
 await tabs.getByRole('tab',{name:'Evidence'}).focus();
 await page.keyboard.press('ArrowRight');
 await expect(tabs.getByRole('tab',{name:'Research'})).toBeFocused();
 await expect(panel.getByText('County parcel number:',{exact:false})).toBeVisible();
 await scrollTab('Research','research');
 await page.keyboard.press('ArrowRight');
 await expectTabRevealed('Notes');
 expect(await tabs.evaluate(el=>el.scrollLeft)).toBeGreaterThan(0);
 await expect(panel.getByRole('region',{name:'Private parcel scratchpad'})).toBeVisible();
 const draft=panel.getByRole('textbox',{name:'Note text'});
 await draft.fill('Retain my unsaved note');
 await page.screenshot({path:info.outputPath('four-tabs-notes.png')});
 await scrollTab('Notes','notes');
 await tabs.getByRole('tab',{name:'Notes'}).focus();
 await page.keyboard.press('ArrowRight');
 await expectTabRevealed('Tasks');
 expect(await tabs.evaluate(el=>el.scrollLeft)).toBeGreaterThan(0);
 await page.screenshot({path:info.outputPath('four-tabs-tasks-horizontal.png')});
 await expect(panel.getByRole('region',{name:'Private parcel scratchpad'})).toBeHidden();
 await expect(panel.getByText('Favorite this parcel to create follow-up tasks.')).toBeVisible();
 await expect(panel.getByRole('button',{name:'Create task'})).toHaveCount(0);
 await page.screenshot({path:info.outputPath('four-tabs-tasks-prerequisite.png')});
 await panel.getByRole('button',{name:'Favorite parcel'}).click();
 const taskPane=panel.getByRole('region',{name:'Parcel follow-up tasks'});
 await expect(taskPane.getByRole('heading',{name:'Follow-up tasks'})).toBeVisible();
 await taskPane.getByRole('textbox',{name:'Follow-up message'}).fill('Retain my unsaved task');
 await taskPane.getByLabel('Due date and time').fill('2030-12-08T14:00');
 await expect(taskPane.getByRole('textbox',{name:'Follow-up message'})).toHaveValue('Retain my unsaved task');
 await page.getByRole('button',{name:'Dismiss notification'}).click();
 await panel.getByRole('tabpanel',{name:'Tasks'}).evaluate(el=>el.scrollTop=0);
 await expect(taskPane.getByRole('heading',{name:'Follow-up tasks'})).toBeVisible();
 await page.screenshot({path:info.outputPath('four-tabs-tasks.png')});
 await scrollTab('Tasks','tasks');
 await tabs.getByRole('tab',{name:'Notes'}).click();
 await expect(draft).toHaveValue('Retain my unsaved note');
 await expect(taskPane).toBeHidden();
 await tabs.getByRole('tab',{name:'Tasks'}).click();
 await expect(taskPane.getByRole('textbox',{name:'Follow-up message'})).toHaveValue('Retain my unsaved task');
 await expect(taskPane.getByLabel('Due date and time')).toHaveValue('2030-12-08T14:00');
});

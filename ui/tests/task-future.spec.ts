import { expect, test, type Page, type Locator } from '@playwright/test';

test.use({timezoneId:'America/New_York'});
const origin='http://127.0.0.1:5174';
const parcel='33333333-3333-4333-8333-333333333333';
const task='44444444-4444-4444-8444-444444444444';
const old={id:task,parcel_id:parcel,parcel_number:'009845',message:'Original reminder',due_at:'2030-12-07T12:00:00-05:00',timezone:'America/New_York',status:'open',reminder_state:'missed',revision:1,schedule_version:1};
async function fixture(page:Page,existing=false,instant='2030-12-08T15:30:20Z'){
 await page.clock.install({time:new Date(instant)});
 await page.emulateMedia({reducedMotion:'reduce'});
 await page.route('**/*',route=>{const u=new URL(route.request().url());if(u.origin!==origin||/^\/(api|auth)(\/|$)/.test(u.pathname))return route.abort();return route.continue()});
 await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
 await page.route('**/api/coverage*',route=>route.fulfill({json:{counties:[]}}));
 await page.route(`**/api/me/favorites/${parcel}`,route=>route.fulfill({json:{parcel_id:parcel,revision:1,favorite:true,excluded:false,exclusion_reason:'',state_id:'11111111-1111-4111-8111-111111111111'}}));
 await page.route(`**/api/me/favorites/${parcel}/notes*`,route=>route.fulfill({json:{items:[],next:''}}));
 await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>route.fulfill({json:{items:existing?[old]:[],next:''}}));
 await page.goto(`/?parcel=${parcel}&tab=tasks`);
 await expect(page.getByRole('textbox',{name:'Follow-up message'})).toBeVisible();
}
async function containment(popup:Locator,selector:string){
 const bounds=await popup.evaluate((el,selectors)=>{
  const rect=(node:Element)=>{const r=node.getBoundingClientRect();return {left:r.left,right:r.right,top:r.top,bottom:r.bottom}};
  const style=getComputedStyle(el), outer=rect(el);
  const inner={left:outer.left+parseFloat(style.borderLeftWidth)+parseFloat(style.paddingLeft),right:outer.right-parseFloat(style.borderRightWidth)-parseFloat(style.paddingRight),top:outer.top+parseFloat(style.borderTopWidth)+parseFloat(style.paddingTop),bottom:outer.bottom-parseFloat(style.borderBottomWidth)-parseFloat(style.paddingBottom)};
  return {outer,inner,scroll:el.scrollWidth,client:el.clientWidth,children:[...el.querySelectorAll(selectors)].filter(e=>e.getBoundingClientRect().width>0).map(rect)};
 },selector);
 const size=popup.page().viewportSize()!;
 expect(bounds.outer.left).toBeGreaterThanOrEqual(0);expect(bounds.outer.right).toBeLessThanOrEqual(size.width);
 expect(bounds.outer.top).toBeGreaterThanOrEqual(0);expect(bounds.outer.bottom).toBeLessThanOrEqual(size.height);
 expect(bounds.scroll).toBeLessThanOrEqual(bounds.client);expect(bounds.children.length).toBeGreaterThan(0);
 for(const child of bounds.children){expect(child.left).toBeGreaterThanOrEqual(bounds.inner.left-1);expect(child.right).toBeLessThanOrEqual(bounds.inner.right+1);expect(child.top).toBeGreaterThanOrEqual(bounds.inner.top-1);expect(child.bottom).toBeLessThanOrEqual(bounds.inner.bottom+1)}
}
async function controlContainment(page:Page){
 const control=page.locator('.date-control');
 const result=await control.evaluate(el=>{
  const box=(e:Element)=>{const r=e.getBoundingClientRect();return {left:r.left,right:r.right,top:r.top,bottom:r.bottom}};
  return {outer:box(el),children:[...el.querySelectorAll('input,button,svg')].map(box),scroll:el.scrollWidth,client:el.clientWidth};
 });
 expect(result.scroll).toBeLessThanOrEqual(result.client);
 for(const child of result.children){expect(child.left).toBeGreaterThanOrEqual(result.outer.left-1);expect(child.right).toBeLessThanOrEqual(result.outer.right+1);expect(child.top).toBeGreaterThanOrEqual(result.outer.top-1);expect(child.bottom).toBeLessThanOrEqual(result.outer.bottom+1)}
}
async function choose(page:Page,h:string,m:string,p:string){const popup=page.getByRole('dialog',{name:'Choose due time'});await page.getByRole('button',{name:'Due time'}).click();await popup.getByLabel('AM or PM').selectOption('PM');await popup.getByLabel('Hour').selectOption(h);await popup.getByLabel('Minute').selectOption(m);await popup.getByLabel('AM or PM').selectOption(p);return popup}
for(const viewport of [{width:1366,height:768},{width:768,height:1024}])test(`future scheduling and contained popups ${viewport.width}x${viewport.height}`,async({page},info)=>{
 test.setTimeout(90000);await page.setViewportSize(viewport);await fixture(page);
 const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});
 await pane.getByRole('textbox',{name:'Follow-up message'}).fill('Follow up');
 const date=pane.getByRole('textbox',{name:'Due date'}), save=pane.getByRole('button',{name:'Create task'});
 await date.fill('2030-12-07');await expect(pane.getByRole('alert')).toContainText('Choose today or a future date');await expect(save).toBeDisabled();
 await page.getByRole('button',{name:'Choose due date'}).click();
 const calendar=page.locator('.calendar-popup');await expect(calendar).toBeVisible();await controlContainment(page);
 await containment(calendar,'[data-scope="date-picker"][data-part="view-control"] *, th, [data-part="table-cell-trigger"], button');
 await expect(calendar.getByText('7',{exact:true})).toHaveAttribute('data-disabled','');
 await page.screenshot({path:info.outputPath('calendar-past-disabled.png')});await calendar.screenshot({path:info.outputPath('calendar-crop.png')});
 await calendar.getByText('8',{exact:true}).click();await expect(date).toHaveValue('2030-12-08');
 let popup=await choose(page,'10','29','AM');
 await containment(popup,'select,button,svg,[data-part="title"]');
 await expect(popup.getByLabel('Hour').locator('option[value="9"]')).toBeEnabled(); // 9 PM remains reachable even when 9 AM is past.
 await popup.getByLabel('Hour').selectOption('9');await expect(popup.getByLabel('AM or PM').locator('option:has-text("AM")')).toBeDisabled();await popup.getByLabel('Hour').selectOption('10');
 await expect(popup.getByLabel('Minute').locator('option[value="29"]')).toBeDisabled();
 await expect(popup.getByLabel('Minute').locator('option[value="31"]')).toBeEnabled();
 await expect(popup.getByRole('button',{name:'Set time'})).toBeDisabled();
 await page.screenshot({path:info.outputPath('time-past-disabled.png')});await popup.screenshot({path:info.outputPath('time-crop.png')});
 await popup.getByLabel('Minute').selectOption('31');await expect(popup.getByRole('button',{name:'Set time'})).toBeEnabled();await popup.getByRole('button',{name:'Set time'}).click();
 await pane.getByLabel(/Time zone offset/).selectOption('2030-12-08T10:31:00-05:00');await expect(save).toBeEnabled();
 await page.clock.fastForward(41_000);await expect(save).toBeDisabled();await expect(pane.getByRole('alert')).toContainText('This time has passed');
 await page.getByRole('button',{name:'Due time'}).click();await expect(page.getByRole('dialog',{name:'Choose due time'}).getByRole('button',{name:'Set time'})).toBeDisabled();
 await page.getByRole('button',{name:'Cancel'}).click();
 await date.fill('2030-12-09');popup=await choose(page,'1','00','AM');await expect(popup.getByLabel('Hour').locator('option[value="1"]')).toBeEnabled();await expect(popup.getByRole('button',{name:'Set time'})).toBeEnabled();await popup.getByRole('button',{name:'Set time'}).click();
 await pane.getByLabel(/Time zone offset/).selectOption('2030-12-09T01:00:00-05:00');await expect(save).toBeEnabled();
});
test('overdue edit requires reschedule but complete and clear remain available',async({page})=>{
 await fixture(page,true);const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});
 await pane.getByRole('button',{name:'Edit / snooze'}).click();await expect(pane.getByRole('button',{name:'Save new schedule version'})).toBeDisabled();
 await expect(pane.getByRole('button',{name:'Complete task'})).toBeEnabled();await expect(pane.getByRole('button',{name:'Clear task'})).toBeEnabled();
 await pane.getByRole('textbox',{name:'Due date'}).fill('2030-12-09');let popup=await choose(page,'1','00','AM');await popup.getByRole('button',{name:'Set time'}).click();await pane.getByLabel(/Time zone offset/).selectOption('2030-12-09T01:00:00-05:00');await expect(pane.getByRole('button',{name:'Save new schedule version'})).toBeEnabled();
});
test('future payload is exact and save checks the instant again without waiting for ticker',async({page})=>{
 await fixture(page);const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});let payload:Record<string,string>|null=null;
 await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>{if(route.request().method()==='POST'){payload=route.request().postDataJSON();return route.fulfill({json:{...payload,status:'open'}})}return route.fulfill({json:{items:[],next:''}})});
 await pane.getByRole('textbox',{name:'Follow-up message'}).fill('Future follow up');await pane.getByRole('textbox',{name:'Due date'}).fill('2030-12-08');
 let popup=await choose(page,'10','32','AM');await popup.getByRole('button',{name:'Set time'}).click();await pane.getByLabel(/Time zone offset/).selectOption('2030-12-08T10:32:00-05:00');await expect(pane.getByRole('button',{name:'Create task'})).toBeEnabled();
 await page.clock.setFixedTime(new Date('2030-12-08T15:32:01Z'));
 await pane.getByRole('button',{name:'Create task'}).click();await expect(pane.getByRole('alert')).toContainText('Choose a future date and time');expect(payload).toBeNull();
 await pane.getByRole('textbox',{name:'Due date'}).fill('2030-12-09');popup=await choose(page,'1','05','AM');await popup.getByRole('button',{name:'Set time'}).click();await pane.getByLabel(/Time zone offset/).selectOption('2030-12-09T01:05:00-05:00');await pane.getByRole('button',{name:'Create task'}).click();
 expect(payload).toMatchObject({message:'Future follow up',due_at:'2030-12-09T01:05:00-05:00',timezone:'America/New_York'});
});
test('fold keeps future offset and rejects past offset; skipped local time cannot be set',async({page})=>{
 await fixture(page,false,'2030-11-03T05:45:00Z');const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});
 await pane.getByRole('textbox',{name:'Follow-up message'}).fill('Fold follow up');await pane.getByRole('textbox',{name:'Due date'}).fill('2030-11-03');
 let popup=await choose(page,'1','30','AM');await expect(popup.getByRole('button',{name:'Set time'})).toBeEnabled();await popup.getByRole('button',{name:'Set time'}).click();
 const offsets=pane.getByLabel(/Time zone offset/);await expect(offsets.locator('option[value="2030-11-03T01:30:00-04:00"]')).toBeDisabled();await expect(offsets.locator('option[value="2030-11-03T01:30:00-05:00"]')).toBeEnabled();
 await offsets.selectOption('2030-11-03T01:30:00-05:00');await expect(pane.getByRole('button',{name:'Create task'})).toBeEnabled();
 await pane.getByRole('textbox',{name:'Due date'}).fill('2031-03-09');popup=await choose(page,'2','30','AM');await expect(popup.getByRole('button',{name:'Set time'})).toBeDisabled();
});
test('calendar minimum moves to the next local day at midnight',async({page})=>{
 await fixture(page,false,'2030-12-09T04:59:59Z');const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});
 await pane.getByRole('textbox',{name:'Due date'}).fill('2030-12-08');await expect(pane.getByRole('alert')).toHaveCount(0);
 await pane.getByRole('button',{name:'Choose due date'}).click();await expect(page.locator('.calendar-popup').getByText('8',{exact:true})).not.toHaveAttribute('data-disabled','');
 await page.clock.fastForward(2_000);await expect(pane.getByRole('alert')).toContainText('Choose today or a future date');
 await expect(page.locator('.calendar-popup').getByText('8',{exact:true})).toHaveAttribute('data-disabled','');
});
test('an open time popup invalidates Set time when the clock passes its minute',async({page})=>{
 await fixture(page);await page.getByRole('textbox',{name:'Due date'}).fill('2030-12-08');
 const popup=await choose(page,'10','31','AM');await expect(popup.getByRole('button',{name:'Set time'})).toBeEnabled();
 await page.clock.fastForward(41_000);await expect(popup.getByRole('button',{name:'Set time'})).toBeDisabled();
 await popup.getByLabel('Minute').selectOption('32');await expect(popup.getByRole('button',{name:'Set time'})).toBeEnabled();
});

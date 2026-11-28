import { expect, test } from '@playwright/test';
test.use({timezoneId:'America/New_York'});
async function chooseTime(page:import('@playwright/test').Page, time:string){
 await page.getByRole('button',{name:'Due time'}).click();
 const popup=page.getByRole('dialog',{name:'Choose due time'});
 await popup.getByLabel('Hour').selectOption(String(Number(time.slice(0,2))%12||12));
 await popup.getByLabel('Minute').selectOption(time.slice(3));
 await popup.getByLabel('AM or PM').selectOption(Number(time.slice(0,2))<12?'AM':'PM');
 await popup.getByRole('button',{name:'Set time'}).click();
}
type Box={x:number;y:number;width:number;height:number};
function contains(parent:Box,child:Box,label:string){
 expect(child.x,label+' left').toBeGreaterThanOrEqual(parent.x-0.5);
 expect(child.y,label+' top').toBeGreaterThanOrEqual(parent.y-0.5);
 expect(child.x+child.width,label+' right').toBeLessThanOrEqual(parent.x+parent.width+0.5);
 expect(child.y+child.height,label+' bottom').toBeLessThanOrEqual(parent.y+parent.height+0.5);
}
async function popupGeometry(popup:import('@playwright/test').Locator,children:string,viewport:{width:number;height:number}){
 const geometry=await popup.evaluate((el,selector)=>{
  const rect=(e:Element)=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}};
  const style=getComputedStyle(el);
  const border=rect(el);
  const content={x:border.x+parseFloat(style.borderLeftWidth)+parseFloat(style.paddingLeft),y:border.y+parseFloat(style.borderTopWidth)+parseFloat(style.paddingTop),width:border.width-parseFloat(style.borderLeftWidth)-parseFloat(style.borderRightWidth)-parseFloat(style.paddingLeft)-parseFloat(style.paddingRight),height:border.height-parseFloat(style.borderTopWidth)-parseFloat(style.borderBottomWidth)-parseFloat(style.paddingTop)-parseFloat(style.paddingBottom)};
  return {border,content,client:el.clientWidth,scroll:el.scrollWidth,children:[...el.querySelectorAll(selector)].map(rect)};
 },children);
 contains({x:0,y:0,...viewport},geometry.border,'popup viewport');
 expect(geometry.scroll,'popup horizontal scroll overflow').toBeLessThanOrEqual(geometry.client);
 expect(geometry.children.length,'popup children').toBeGreaterThan(0);
 geometry.children.forEach((child,i)=>contains(geometry.content,child,`popup child ${i}`));
 return geometry;
}
const origin='http://127.0.0.1:5174';
const parcel='33333333-3333-4333-8333-333333333333',other='55555555-5555-4555-8555-555555555555',state='11111111-1111-4111-8111-111111111111';
const due='2030-12-08T14:00:00Z';
const task=(id:string,parcelID:string)=>({id,parcel_id:parcelID,parcel_number:parcelID===other?null:'009845',message:'Fixture follow up',due_at:due,timezone:'UTC',status:'open',reminder_state:'queued',revision:1,schedule_version:1});
async function fixture(page:import('@playwright/test').Page){
await page.emulateMedia({reducedMotion:'reduce'});
await page.route('**/*',route=>{const u=new URL(route.request().url());if(u.origin!==origin||/^\/(api|auth)(\/|$)/.test(u.pathname))return route.abort();return route.continue()});
await page.route('**/api/me',route=>route.fulfill({json:{id:'buyer-fixture',email:'buyer@example.test',name:'Buyer'}}));
await page.route('**/api/coverage*',route=>route.fulfill({json:{counties:[]}}));
await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:state,name:'New',position:0,revision:1,count:1}],default_state_id:state,revision:1,next:''}}));
await page.route(`**/api/me/crm/board/${state}*`,route=>route.fulfill({json:{items:[{parcel_id:parcel,parcel_number:'009845',address:'10 Fixture Lane',state_id:state,favorite:true,excluded:false,exclusion_reason:'',revision:1,favorited_at:'2026-11-24 12:00:00+00'}],next:''}}));
await page.route(`**/api/me/favorites/${parcel}`,route=>route.fulfill({json:{parcel_id:parcel,revision:1,favorite:false,excluded:false,exclusion_reason:'',state_id:state}}));
await page.route(`**/api/me/favorites/${parcel}/notes*`,route=>route.fulfill({json:{items:[],next:''}}));
}
test('board Add task selects nonfavorite parcel Tasks tab and global queue stays on /tasks',async({page},info)=>{
await fixture(page);let reads=0;
await page.route('**/api/me/crm/tasks*',route=>{reads++;return route.fulfill({json:{items:[task('44444444-4444-4444-8444-444444444444',parcel)],next:''}})});
await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>route.fulfill({json:{items:[],next:''}}));
await page.goto('/leads');await expect(page.getByRole('heading',{name:'Leads'})).toBeVisible();
await expect(page.getByRole('region',{name:'Follow-up task queue'})).toHaveCount(0);expect(reads).toBe(0);
await page.screenshot({path:info.outputPath('01-leads-board-no-queue.png'),fullPage:true});
await page.getByRole('button',{name:'Actions for parcel 009845'}).click();await page.getByRole('menuitem',{name:'Add task'}).click();
await expect(page).toHaveURL(new RegExp(`parcel=${parcel}.*tab=tasks`));
await expect(page.getByRole('tab',{name:'Tasks'})).toHaveAttribute('aria-selected','true');
await expect(page.getByRole('tab',{name:'Tasks'})).toBeInViewport();
await expect(page.getByRole('button',{name:'Favorite parcel'})).toHaveAttribute('aria-pressed','false');
await expect(page.getByRole('textbox',{name:'Follow-up message'})).toBeVisible();
await page.screenshot({path:info.outputPath('02-nonfavorite-tasks-tab.png'),fullPage:true});
await page.getByRole('link',{name:'Tasks',exact:true}).click();
await expect(page.getByRole('heading',{name:'Tasks',exact:true})).toBeVisible();
await expect(page.getByRole('link',{name:/Parcel 009845/})).toHaveAttribute('href',`/?parcel=${parcel}&tab=tasks`);
await page.screenshot({path:info.outputPath('03-global-tasks.png'),fullPage:true});expect(reads).toBeGreaterThan(0);
});
test('parcel Clear all requires confirmation and affects only that parcel',async({page},info)=>{
await fixture(page);let cleared=false,posts=0;const first=task('44444444-4444-4444-8444-444444444444',parcel);
await page.route(`**/api/me/parcels/${parcel}/tasks/clear`,route=>{
if(route.request().method()==='POST'){expect(route.request().postDataJSON()).toEqual({count:1,snapshot:'0123456789abcdef0123456789abcdef'});posts++;cleared=true;return route.fulfill({json:{canceled:1}})}
return route.fulfill({json:{count:cleared?0:1,snapshot:'0123456789abcdef0123456789abcdef'}});
});
await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>route.fulfill({json:{items:cleared?[]:[first],next:''}}));
await page.route('**/api/me/crm/tasks*',route=>route.fulfill({json:{items:cleared?[task(other,other)]:[first,task(other,other)],next:''}}));
await page.goto(`/?parcel=${parcel}&tab=tasks`);await expect(page.getByText('Fixture follow up')).toBeVisible();
await page.getByRole('button',{name:'Clear all tasks…'}).click();await expect(page.getByRole('group',{name:'Confirm clear all tasks'})).toContainText('Cancel 1 open tasks on this parcel');
await page.screenshot({path:info.outputPath('04-clear-confirmation.png'),fullPage:true});
await page.getByRole('button',{name:'Keep tasks'}).click();expect(posts).toBe(0);await expect(page.getByText('Fixture follow up')).toBeVisible();
await page.getByRole('button',{name:'Clear all tasks…'}).click();await page.getByRole('button',{name:'Confirm clear all'}).click();expect(posts).toBe(1);
await expect(page.getByText('No open tasks.')).toHaveCount(0);
await page.getByRole('link',{name:'Tasks',exact:true}).click();await expect(page.getByRole('link',{name:new RegExp(other)})).toBeVisible();
await expect(page.getByRole('link',{name:/Parcel 009845/})).toHaveCount(0);
await page.screenshot({path:info.outputPath('05-other-parcel-survives.png'),fullPage:true});
});
test('first task retains nonfavorite revision so the first star succeeds once',async({page},info)=>{
await fixture(page);let created=false,reads=0,writes=0, selectedDue='',selectedZone='';
await page.route(`**/api/me/favorites/${parcel}`,route=>{
 if(route.request().method()==='GET') {reads++;return created?route.fulfill({json:{parcel_id:parcel,revision:1,favorite:false,excluded:false,exclusion_reason:'',state_id:state}}):route.fulfill({status:404})}
 expect(route.request().method()).toBe('PUT');expect(route.request().postDataJSON()).toEqual({revision:1});writes++;
 return route.fulfill({json:{parcel_id:parcel,revision:2,favorite:true,excluded:false,exclusion_reason:'',state_id:state}});
});
await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>{
 if(route.request().method()==='POST') {expect(route.request().postDataJSON()).toMatchObject({message:'First follow up',due_at:selectedDue,timezone:selectedZone});created=true;return route.fulfill({json:task('675d364c-70f0-4468-b979-422df800fbce',parcel)})}
 return route.fulfill({json:{items:created?[task('675d364c-70f0-4468-b979-422df800fbce',parcel)]:[],next:''}});
});
await page.goto(`/?parcel=${parcel}&tab=tasks`);await expect(page.getByRole('button',{name:'Favorite parcel'})).toBeEnabled();
await page.getByRole('textbox',{name:'Follow-up message'}).fill('First follow up');
await page.getByLabel('Due date',{exact:true}).fill('2030-12-08');
await page.getByRole('button',{name:'Choose due date'}).click();
await page.locator('.calendar-popup').getByText('9',{exact:true}).click();
await expect(page.getByLabel('Due date',{exact:true})).toHaveValue('2030-12-09');
await chooseTime(page,'14:37');
selectedZone=await page.evaluate(()=>Intl.DateTimeFormat().resolvedOptions().timeZone);
selectedDue='2030-12-09T14:37:00-05:00';
await page.getByLabel(/Time zone offset/).selectOption(selectedDue);
await page.getByRole('button',{name:'Create task'}).click();
await expect(page.getByRole('status').filter({hasText:'Task saved.'})).toBeVisible();
await expect(page.getByRole('button',{name:'Favorite parcel'})).toBeEnabled();
expect(reads).toBe(2);await page.getByRole('button',{name:'Favorite parcel'}).click();
await expect(page.getByRole('button',{name:'Remove favorite'})).toHaveAttribute('aria-pressed','true');expect(writes).toBe(1);
await page.screenshot({path:info.outputPath('06-first-task-then-favorite.png'),fullPage:true});
});
test('excluded parcel can clear existing tasks but cannot schedule or simulate',async({page},info)=>{
await fixture(page);let cleared=false,posts=0;
await page.route(`**/api/me/favorites/${parcel}`,route=>route.fulfill({json:{parcel_id:parcel,revision:2,favorite:true,excluded:true,exclusion_reason:'Excluded fixture',state_id:state}}));
await page.route(`**/api/me/parcels/${parcel}/tasks/clear`,route=>{
 if(route.request().method()==='POST'){expect(route.request().postDataJSON()).toEqual({count:1,snapshot:'0123456789abcdef0123456789abcdef'});posts++;cleared=true;return route.fulfill({json:{canceled:1}})}
 return route.fulfill({json:{count:cleared?0:1,snapshot:'0123456789abcdef0123456789abcdef'}});
});
await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>route.fulfill({json:{items:cleared?[]:[{...task('a4183a27-8402-4f42-9773-4525de25399f',parcel),due_at:'2020-12-08T14:00:00Z'}],next:''}}));
await page.goto(`/?parcel=${parcel}&tab=tasks`);await expect(page.getByText('Fixture follow up')).toBeVisible();
await expect(page.getByRole('textbox',{name:'Follow-up message'})).toHaveCount(0);
await expect(page.getByRole('button',{name:'Edit / snooze'})).toHaveCount(0);
await expect(page.getByRole('button',{name:/Simulate SMS/})).toHaveCount(0);
await expect(page.getByRole('button',{name:'Complete task'})).toBeVisible();
await page.getByRole('button',{name:'Clear all tasks…'}).click();await expect(page.getByRole('group',{name:'Confirm clear all tasks'})).toBeVisible();
await page.screenshot({path:info.outputPath('07-excluded-clear-confirmation.png'),fullPage:true});
await page.getByRole('button',{name:'Confirm clear all'}).click();await expect(page.getByText('No open tasks.')).toHaveCount(0);expect(posts).toBe(1);
});
test('Clear all rejects changed confirmation and keeps refreshed tasks',async({page})=>{
await fixture(page);let attempts=0;
await page.route(`**/api/me/parcels/${parcel}/tasks/clear`,route=>{
if(route.request().method()==='POST'){attempts++;return route.fulfill({status:409})}
return route.fulfill({json:{count:1,snapshot:'0123456789abcdef0123456789abcdef'}});
});
await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>route.fulfill({json:{items:[task(other,parcel)],next:''}}));
await page.goto(`/?parcel=${parcel}&tab=tasks`);
await expect(page.getByText('Fixture follow up')).toBeVisible();
await page.getByRole('button',{name:'Clear all tasks…'}).click();
await page.getByRole('button',{name:'Confirm clear all'}).click();
await expect(page.getByRole('region',{name:'Parcel follow-up tasks'}).getByRole('alert')).toContainText('Task count changed');
await expect(page.getByRole('group',{name:'Confirm clear all tasks'})).toHaveCount(0);
await expect(page.getByText('Fixture follow up')).toBeVisible();expect(attempts).toBe(1);
});
test('task editor fits date and time controls in narrow pane and enforces one segment',async({page},info)=>{
await fixture(page);
let posts=0;
await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>{
 if(route.request().method()==='POST'){posts++;return route.fulfill({status:500})}
 return route.fulfill({json:{items:[],next:''}});
});
for(const viewport of [{width:1440,height:900},{width:1366,height:768},{width:768,height:1024},{width:600,height:800}]){
 await page.setViewportSize(viewport);
 await page.goto(`/?parcel=${parcel}&tab=tasks`);
 const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});
 const date=pane.getByLabel('Due date',{exact:true}),time=pane.getByRole('button',{name:'Due time'});
 await expect(date).toBeVisible();await expect(time).toBeVisible();
 const panelBox=await pane.boundingBox(),dateBox=await date.boundingBox(),timeBox=await time.boundingBox();
 const control=pane.locator('.date-control'),trigger=pane.getByRole('button',{name:'Choose due date'});
 const controlBox=await control.boundingBox(),triggerBox=await trigger.boundingBox(),iconBox=await trigger.locator('svg').boundingBox();
 expect(panelBox).not.toBeNull();expect(dateBox).not.toBeNull();expect(timeBox).not.toBeNull();
 expect(controlBox).not.toBeNull();expect(triggerBox).not.toBeNull();expect(iconBox).not.toBeNull();
 contains(panelBox!,controlBox!,'date control in parcel pane');
 contains(controlBox!,dateBox!,'date input in control');
 contains(controlBox!,triggerBox!,'calendar trigger in control');
 contains(triggerBox!,iconBox!,'calendar icon in trigger');
 contains(panelBox!,timeBox!,'time trigger in parcel pane');
 expect(await control.evaluate(el=>el.scrollWidth)).toBeLessThanOrEqual(await control.evaluate(el=>el.clientWidth));
 await date.fill('2030-12-08');
 await pane.getByRole('button',{name:'Choose due date'}).click();
 const calendar=page.locator('.calendar-popup');await expect(calendar).toBeVisible();
 await expect.poll(async()=>{const b=await calendar.boundingBox();return !!b&&b.x>=0&&b.y>=0&&b.x+b.width<=viewport.width&&b.y+b.height<=viewport.height}).toBe(true);
 const calendarBox=await popupGeometry(calendar,'table, .calendar-heading, .calendar-heading button',viewport);
 const table=calendar.locator('table');
 expect(await table.evaluate(el=>el.scrollWidth)).toBeLessThanOrEqual(await table.evaluate(el=>el.clientWidth));
 const weekdays=calendar.locator('thead th');await expect(weekdays).toHaveCount(7);
 const rows=calendar.locator('tbody tr');expect(await rows.count()).toBeGreaterThanOrEqual(4);
 for(const row of await rows.all())await expect(row.locator('td > [data-part="table-cell-trigger"]')).toHaveCount(7);
 for(const [label,locator] of [['weekday',weekdays],['day cell',calendar.locator('tbody td')],['day trigger',calendar.locator('tbody td > [data-part="table-cell-trigger"]')]] as const){
  for(const [index,child] of (await locator.all()).entries())contains(calendarBox.content,(await child.boundingBox())!,`${label} ${index}`);
 }
 for(const [name,popup] of [['calendar',calendar],['time',page.getByRole('dialog',{name:'Choose due time'})]] as const){
  if(name==='time')await time.click();
  await expect(popup).toBeVisible();
  if(name==='time'){
   await expect.poll(async()=>{const b=await popup.boundingBox();return !!b&&b.x>=0&&b.y>=0&&b.x+b.width<=viewport.width&&b.y+b.height<=viewport.height}).toBe(true);
   await popupGeometry(popup,'select, button',viewport);
  }
  await page.screenshot({path:info.outputPath(`${name}-open-${viewport.width}x${viewport.height}.png`)});
  if(name==='calendar')await pane.getByRole('button',{name:'Choose due date'}).click();
  else {await popup.getByLabel('Hour').selectOption('7');await popup.press('Escape');await expect(time).toBeFocused();await expect(time).toContainText('Choose time');}
 }
 await chooseTime(page,'14:00');
 await expect(pane.getByLabel(/Time zone offset/)).toBeVisible();
 await pane.getByRole('textbox',{name:'Follow-up message'}).fill('Plain follow up');
 await expect(pane.getByText('145 characters remaining')).toBeVisible();
 await expect(pane.getByText(/GSM-7|units remaining|No open tasks/)).toHaveCount(0);
 await pane.screenshot({path:info.outputPath(`task-controls-${viewport.width}x${viewport.height}.png`)});
 }
const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});
const message=pane.getByRole('textbox',{name:'Follow-up message'});
await message.fill('A^');
await expect(pane.getByText('157 characters remaining')).toBeVisible();
await message.fill('A😀');
await expect(pane.getByText('67 characters remaining')).toBeVisible();
await message.fill('^'.repeat(81));
await expect(pane.getByRole('alert')).toContainText('Message is too long for one SMS');
await expect(pane.getByRole('button',{name:'Create task'})).toBeDisabled();
await message.fill('😀'.repeat(36));
await expect(pane.getByRole('button',{name:'Create task'})).toBeDisabled();
await message.fill('^'.repeat(80));
await expect(pane.getByText('0 characters remaining')).toBeVisible();
await expect(pane.getByRole('alert')).toHaveCount(0);
await pane.getByLabel(/Time zone offset/).selectOption({index:1});
await pane.getByLabel('Due date',{exact:true}).fill('2030-12-09');
await expect(pane.getByLabel(/Time zone offset/)).toHaveValue('');
await expect(pane.getByRole('button',{name:'Create task'})).toBeDisabled();
await page.getByLabel('Due date',{exact:true}).fill('');
await expect(pane.getByLabel(/Time zone offset/)).toHaveCount(0);
await expect(pane.getByRole('button',{name:'Create task'})).toBeDisabled();
expect(posts).toBe(0);
});
test('rightmost calendar day schedules the selected local instant',async({page},info)=>{
 await fixture(page);
 let submitted=false;
 await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>{
  if(route.request().method()==='POST'){
   expect(route.request().postDataJSON()).toMatchObject({message:'Saturday follow up',due_at:'2030-12-14T14:37:00-05:00',timezone:'America/New_York'});
   submitted=true;return route.fulfill({json:task('675d364c-70f0-4468-b979-422df800fbce',parcel)});
  }
  return route.fulfill({json:{items:[],next:''}});
 });
 await page.setViewportSize({width:1366,height:768});
 await page.goto(`/?parcel=${parcel}&tab=tasks`);
 await page.getByRole('textbox',{name:'Follow-up message'}).fill('Saturday follow up');
 await page.getByLabel('Due date',{exact:true}).fill('2030-12-08');
 await page.getByRole('button',{name:'Choose due date'}).click();
 const calendar=page.locator('.calendar-popup');await expect(calendar).toBeVisible();
 await expect(calendar.locator('thead th')).toHaveCount(7);
 const saturday=calendar.locator('tbody tr').nth(1).locator('td').last().locator('[data-part="table-cell-trigger"]');
 await expect(saturday).toHaveText('14');
 await saturday.click();
 await expect(page.getByLabel('Due date',{exact:true})).toHaveValue('2030-12-14');
 await chooseTime(page,'14:37');
 await page.getByLabel(/Time zone offset/).selectOption('2030-12-14T14:37:00-05:00');
 await page.getByRole('button',{name:'Create task'}).click();
 await expect(page.getByRole('status').filter({hasText:'Task saved.'})).toBeVisible();
 expect(submitted).toBe(true);
 await page.screenshot({path:info.outputPath('saturday-submitted-1366x768.png')});
});
test('edit keeps the loaded fold instant and validates skipped hours before rescheduling',async({page})=>{
 await fixture(page);
 const id='44444444-4444-4444-8444-444444444444';
 let saved=false;
 const loaded={...task(id,parcel),due_at:'2030-11-03T06:30:00Z',timezone:'America/New_York'};
 await page.route(`**/api/me/parcels/${parcel}/tasks/${id}/reschedule`,route=>{
  expect(route.request().postDataJSON()).toEqual({revision:1,message:'Edited reminder',due_at:'2030-11-03T02:45:00-05:00',timezone:'America/New_York'});
  saved=true;return route.fulfill({json:{...loaded,revision:2,schedule_version:2}});
 });
 await page.route(`**/api/me/parcels/${parcel}/tasks*`,route=>route.fulfill({json:{items:saved?[]:[loaded],next:''}}));
 await page.goto(`/?parcel=${parcel}&tab=tasks`);
 const pane=page.getByRole('region',{name:'Parcel follow-up tasks'});
 await pane.getByRole('button',{name:'Edit / snooze'}).click();
 await expect(pane.getByLabel('Due date',{exact:true})).toHaveValue('2030-11-03');
 await expect(pane.getByRole('button',{name:'Due time'})).toContainText('01:30');
 await expect(pane.getByLabel(/Time zone offset/)).toHaveValue('2030-11-03T01:30:00-05:00');
 await expect(pane.getByLabel(/Time zone offset/).locator('option:not([disabled])')).toHaveCount(2);
 await pane.getByLabel('Due date',{exact:true}).fill('2030-03-10');
 await chooseTime(page,'02:30');
 await expect(pane.getByRole('alert')).toContainText('does not exist');
 await expect(pane.getByRole('button',{name:'Save new schedule version'})).toBeDisabled();
 await pane.getByLabel('Due date',{exact:true}).fill('2030-02-30');
 await expect(pane.getByRole('button',{name:'Save new schedule version'})).toBeDisabled();
 await pane.getByLabel('Due date',{exact:true}).fill('2030-11-03');
 await chooseTime(page,'02:45');
 await pane.getByRole('textbox',{name:'Follow-up message'}).fill('Edited reminder');
 await pane.getByLabel(/Time zone offset/).selectOption('2030-11-03T02:45:00-05:00');
 await pane.getByRole('button',{name:'Save new schedule version'}).click();
 await expect(pane.getByText('Task saved.',{exact:false})).toBeVisible();expect(saved).toBe(true);
});

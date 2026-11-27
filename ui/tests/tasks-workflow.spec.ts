import { expect, test } from '@playwright/test';
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
await expect(page.getByText('No open tasks.')).toBeVisible();
await page.getByRole('link',{name:'Tasks',exact:true}).click();await expect(page.getByRole('link',{name:new RegExp(other)})).toBeVisible();
await expect(page.getByRole('link',{name:/Parcel 009845/})).toHaveCount(0);
await page.screenshot({path:info.outputPath('05-other-parcel-survives.png'),fullPage:true});
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
await expect(page.getByRole('alert')).toContainText('Task count changed');
await expect(page.getByRole('group',{name:'Confirm clear all tasks'})).toHaveCount(0);
await expect(page.getByText('Fixture follow up')).toBeVisible();expect(attempts).toBe(1);
});

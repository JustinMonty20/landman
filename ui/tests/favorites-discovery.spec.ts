import { expect, test } from '@playwright/test';
const origin='http://127.0.0.1:5174';
const a='11111111-1111-4111-8111-111111111111', b='22222222-2222-4222-8222-222222222222';
const card=(id:string,favorite:boolean,revision:number)=>({parcel_id:id,favorite,state_id:a,revision,excluded:false,exclusion_reason:''});
async function fixture(page:import('@playwright/test').Page){
 await page.route('**/*',route=>{const u=new URL(route.request().url());if(u.origin!==origin||/^\/(api|auth)(\/|$)/.test(u.pathname))return route.abort();return route.continue()});
 await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
 await page.route('**/api/coverage*',route=>route.fulfill({json:{counties:[]}}));
}
test('star toggles confirmed favorite, toast navigates to Leads; removal retains revision',async ({page},info)=>{
 await fixture(page);let saved=false,revision=1;
 await page.route(`**/api/me/favorites/${a}`,route=>{
  if(route.request().method()==='GET')return route.fulfill({status:saved?200:404,json:saved?card(a,true,revision):{}});
  if(route.request().method()==='DELETE'){expect(route.request().postDataJSON()).toEqual({revision});saved=false;revision++;return route.fulfill({json:card(a,false,revision)})}
  expect(route.request().postDataJSON()).toEqual(saved?{revision}:revision===1?{}:{revision});saved=true;revision++;return route.fulfill({json:card(a,true,revision)})
 });
 await page.route('**/api/me/crm/board',route=>route.fulfill({json:{states:[{id:a,name:'New',position:0,revision:1,count:1}],default_state_id:a,revision:1,next:''}}));
 await page.route('**/api/me/crm/board/*',route=>route.fulfill({json:{items:[],next:''}}));
 await page.goto(`/?parcel=${a}`);const star=page.getByRole('button',{name:'Favorite parcel'});await expect(star).toHaveAttribute('aria-pressed','false');await expect(page.getByRole('link',{name:'Leads'})).toHaveAttribute('href','/leads');
 await page.screenshot({path:info.outputPath('neutral-star.png')});await star.click();await expect(page.getByRole('button',{name:'Remove favorite'})).toHaveAttribute('aria-pressed','true');await expect(page.getByRole('button',{name:'View board'})).toBeVisible();await expect.poll(async()=>{const box=await page.getByRole('button',{name:'View board'}).boundingBox();return box ? box.y+box.height : 2000}).toBeLessThan(info.project.use.viewport!.height-12);await page.screenshot({path:info.outputPath('saved-toast.png')});
 await page.getByRole('button',{name:'Remove favorite'}).click();await expect(star).toHaveAttribute('aria-pressed','false');await expect(page.getByText(/history are retained/)).toBeVisible();await star.click();await expect(page.getByRole('button',{name:'Remove favorite'})).toBeVisible();await page.getByRole('button',{name:'View board'}).last().click();await expect(page).toHaveURL(/\/leads$/);await expect(page.getByRole('heading',{name:'Leads'})).toBeVisible();
});
test('failed write never paints star gold; stale parcel response does not alter new selection',async ({page})=>{
 await fixture(page);let release:(()=>void)|undefined;let fail=true;
 await page.route(`**/api/me/favorites/${a}`,async route=>{if(route.request().method()==='GET')return route.fulfill({status:404});if(fail){fail=false;return route.fulfill({status:503})}await new Promise<void>(resolve=>release=resolve);return route.fulfill({json:card(a,true,1)})});
 await page.route(`**/api/me/favorites/${b}`,route=>route.fulfill({status:404}));
 await page.goto(`/?parcel=${a}`);await page.getByRole('button',{name:'Favorite parcel'}).click();await expect(page.getByRole('alert')).toContainText('Could not save favorite');await expect(page.getByRole('button',{name:'Favorite parcel'})).toHaveAttribute('aria-pressed','false');
 await page.getByRole('button',{name:'Favorite parcel'}).click();await expect(page.getByRole('button',{name:'Favorite parcel'})).toBeDisabled();await page.goto(`/?parcel=${b}`);await expect(page.getByRole('button',{name:'Favorite parcel'})).toBeEnabled();release?.();await expect(page.getByRole('button',{name:'Favorite parcel'})).toHaveAttribute('aria-pressed','false');await expect(page.getByRole('button',{name:'View board'})).toHaveCount(0);
});

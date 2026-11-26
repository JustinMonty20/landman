import { expect, test } from '@playwright/test';
const origin='http://127.0.0.1:5174';
const a='11111111-1111-4111-8111-111111111111', b='22222222-2222-4222-8222-222222222222';
const card=(id:string,favorite:boolean,revision:number)=>({parcel_id:id,favorite,state_id:a,revision,excluded:false,exclusion_reason:''});
async function fixture(page:import('@playwright/test').Page){
 await page.route('**/*',route=>{const u=new URL(route.request().url());if(u.origin!==origin||/^\/(api|auth)(\/|$)/.test(u.pathname))return route.abort();return route.continue()});
 await page.route('**/api/me',route=>route.fulfill({json:{id:'fixture',email:'buyer@example.test',name:'Buyer'}}));
await page.route('**/api/me/favorites/*/notes*',route=>route.fulfill({json:{items:[],next:''}}));
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

async function selectViaFinder(page:import('@playwright/test').Page,id:string){
 const finder=page.getByRole('combobox',{name:'Find parcel ID or address'});
 await finder.fill('SWITCH');
 await page.getByRole('option',{name:`Switch · ${id.slice(0,8)}`}).click();
}
for(const removing of [false,true]) test(`mounted finder switch ignores late ${removing?'remove':'save'} body`,async ({page})=>{
 await fixture(page);
 await page.addInitScript(({id})=>{
  const original=Response.prototype.json;
  (window as any).releaseFavoriteBody=()=>{(window as any).favoriteBodyRelease?.()};
  Response.prototype.json=async function(){

   if(this.url.includes(`/api/me/favorites/${id}`) && this.status===200 && (this as any).__favoriteWrite){
    await new Promise<void>(resolve=>(window as any).favoriteBodyRelease=resolve);
   }
   return original.call(this);
  };
  const originalFetch=window.fetch;
  window.fetch=async (...args)=>{
   const response=await originalFetch(...args);
   if(String(args[0]).includes(`/api/me/favorites/${id}`) && args[1]?.method && args[1].method!=='GET') (response as any).__favoriteWrite=true;
   return response;
  };
 },{id:a});
 await page.route('**/api/v1/parcels/lookup*',route=>route.fulfill({json:{items:[{id:b,parcel_number:b.slice(0,8),address:'Switch'}]}}));
 await page.route(`**/api/me/favorites/${a}`,route=>route.fulfill({status:200,json:card(a,removing,7)}));
 await page.route(`**/api/me/favorites/${b}`,route=>route.fulfill({status:200,json:card(b,false,19)}));
 await page.goto(`/?parcel=${a}`);
 const button=page.getByRole('button',{name:removing?'Remove favorite':'Favorite parcel'});
 await expect(button).toBeEnabled();await button.click();
 await expect(button).toBeDisabled();
 await selectViaFinder(page,b);
 await expect(page.getByRole('button',{name:'Favorite parcel'})).toBeEnabled();
 await page.evaluate(()=>(window as any).releaseFavoriteBody());
 await expect(page.getByRole('button',{name:'Favorite parcel'})).toHaveAttribute('aria-pressed','false');
 await expect(page.getByRole('button',{name:'View board'})).toHaveCount(0);
 await expect(page.getByText(/history are retained/)).toHaveCount(0);
});

test('favorite reads block writes until known, retry failure and retained reactivation use revision',async ({page})=>{
 await fixture(page);
 let release:(()=>void)|undefined, mode:'delayed'|'error'|'retained'|'absent'='delayed',writes=0;
 await page.route(`**/api/me/favorites/${a}`,async route=>{
  if(route.request().method()==='GET'){
   if(mode==='delayed') await new Promise<void>(resolve=>release=resolve);
   if(mode==='error')return route.fulfill({status:503});
   if(mode==='absent')return route.fulfill({status:404});
   return route.fulfill({json:card(a,false,13)});
  }
  writes++;expect(route.request().postDataJSON()).toEqual({revision:13});
  return route.fulfill({json:card(a,true,14)});
 });
 await page.goto(`/?parcel=${a}`);
 const star=page.getByRole('button',{name:'Favorite parcel'});
 await expect(star).toBeDisabled();await star.evaluate((el:HTMLElement)=>el.click());expect(writes).toBe(0);
 mode='error';release?.();await expect(page.getByRole('button',{name:'Retry favorite status'})).toBeEnabled();expect(writes).toBe(0);
 mode='retained';await page.getByRole('button',{name:'Retry favorite status'}).click();await expect(star).toBeEnabled();await expect(page.getByRole('button',{name:'View board'})).toHaveCount(0);
 await star.click();await expect(page.getByRole('button',{name:'Remove favorite'})).toBeVisible();expect(writes).toBe(1);
});

test('saved GET and missing GET stay disabled until settled with no mount toast',async ({page})=>{
 await fixture(page);let release:(()=>void)|undefined, get=0,writes=0;
 await page.route(`**/api/me/favorites/${a}`,async route=>{
  if(route.request().method()!=='GET'){writes++;return route.fulfill({status:503})}
  get++;await new Promise<void>(resolve=>release=resolve);
  return route.fulfill(get===1?{json:card(a,true,8)}:{status:404});
 });
 await page.goto(`/?parcel=${a}`);await expect(page.getByRole('button',{name:'Favorite parcel'})).toBeDisabled();
 await expect.poll(()=>!!release).toBe(true);release?.();
 await expect(page.getByRole('button',{name:'Remove favorite'})).toBeEnabled();await expect(page.getByRole('button',{name:'View board'})).toHaveCount(0);
 await page.reload();await expect(page.getByRole('button',{name:'Favorite parcel'})).toBeDisabled();await expect.poll(()=>get).toBe(2);release?.();await expect(page.getByRole('button',{name:'Favorite parcel'})).toBeEnabled();expect(writes).toBe(0);
});

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

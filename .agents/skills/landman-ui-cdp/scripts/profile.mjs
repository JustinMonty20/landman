import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

export async function profileAction(page, action) {
  const cdp = await page.context().newCDPSession(page);
  try {
    await cdp.send('Profiler.enable');
    await cdp.send('Performance.enable');
    const { metrics: before } = await cdp.send('Performance.getMetrics');
    await cdp.send('Profiler.start');
    const start = performance.now();
    await action();
    const elapsedMs = performance.now() - start;
    const { profile } = await cdp.send('Profiler.stop');
    const { metrics: after } = await cdp.send('Performance.getMetrics');
    const dir = await mkdtemp(join(tmpdir(), 'landman-ui-cdp-'));
    try {
      const options = { mode: 0o600, flag: 'wx' };
      await writeFile(join(dir, 'action.cpuprofile'), JSON.stringify(profile), options);
      await writeFile(join(dir, 'metrics.json'), JSON.stringify({ elapsedMs, before, after }, null, 2), options);
      return dir;
    } catch (error) {
      await rm(dir, { recursive: true, force: true });
      throw error;
    }
  } finally {
    await cdp.detach();
  }
}

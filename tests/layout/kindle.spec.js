const { test, expect } = require('@playwright/test');

for (const scenario of ['', 'long', 'minimal']) {
  test(`Kindle layout fits: ${scenario || 'photo summary'}`, async ({ page }, testInfo) => {
    const landscape = testInfo.project.name.includes('landscape');
    await page.goto(`/?${landscape ? 'h&' : ''}${scenario}`);
    await page.evaluate(() => document.fonts.ready);
    const problems = await page.evaluate(() => {
      const errors = [];
      // Bands may sit side by side in landscape, so check every pair of rows.
      const rows = ['#current-weather', '#rain-band', '#tide-band', '#beach-status', '#insights', '#footer']
        .map(selector => [selector, document.querySelector(selector)])
        .filter(([, element]) => element)
        .map(([selector, element]) => [selector, element.getBoundingClientRect()]);
      for (let i = 0; i < rows.length; i++) {
        for (let j = i + 1; j < rows.length; j++) {
          const [a, A] = rows[i], [b, B] = rows[j];
          if (A.left < B.right - 1 && B.left < A.right - 1 && A.top < B.bottom - 1 && B.top < A.bottom - 1)
            errors.push(`${b} overlaps ${a}`);
        }
      }
      const bottom = Math.max(...rows.map(([, box]) => box.bottom));
      if (bottom > innerHeight) errors.push(`overview ends at ${bottom}, viewport ends at ${innerHeight}`);
      if (document.documentElement.scrollWidth > innerWidth) errors.push('horizontal overflow');
      // Check individual labels as well as row boxes: clipped text can fit inside a broken row.
      for (const element of document.querySelectorAll('#temp, #now, .band-main, .band-side, .band-label, .band-big, .band-caption, .band-text, .insight, #moon, #sun, #launch-cell')) {
        if (element.scrollWidth > element.clientWidth + 1)
          errors.push(`clipped content: ${element.className || element.id}`);
      }
      const temp = document.querySelector('#temp').getBoundingClientRect();
      const launch = document.querySelector('#launches');
      if (launch && launch.getBoundingClientRect().top < temp.bottom) errors.push('launch overlaps temperature');
      if (!document.fonts.check('32px weathericons')) errors.push('weather icon font unavailable');
      return errors;
    });
    expect(problems).toEqual([]);
  });
}

test('detail links and tables work without JavaScript', async ({ browser }, testInfo) => {
  const landscape = testInfo.project.name.includes('landscape');
  const context = await browser.newContext({ javaScriptEnabled: false, viewport: testInfo.project.use.viewport });
  const page = await context.newPage();
  await page.goto(`/?${landscape ? 'h&' : ''}`);
  for (const [label, id] of [['Surf', 'surf'], ['Tides', 'tides'], ['Rain', 'rain'], ['Overview', 'overview']]) {
    await page.locator('nav').first().getByRole('link', { name: label, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`section=${id}#${id}$`));
    if (landscape) expect(new URL(page.url()).searchParams.has('h')).toBeTruthy();
    const section = page.locator(`#${id}`);
    const box = await section.boundingBox();
    // The final section may sit below the top when the page cannot scroll farther.
    expect(box.y).toBeGreaterThanOrEqual(-1);
    expect(box.y).toBeLessThan(testInfo.project.use.viewport.height - 40);
    await expect(page.locator('meta[http-equiv="refresh"]')).toHaveAttribute('content', new RegExp(`section=${id}#${id}$`));
  }
  await context.close();
});

test('detail tables stay readable within the page width', async ({ page }) => {
  await page.goto('/');
  const problems = await page.evaluate(() => {
    const errors = [];
    for (const element of document.querySelectorAll('.detail-table th, .detail-table td, .detail-nav a, .detail-section')) {
      if (element.scrollWidth > element.clientWidth + 1) errors.push(`overflow: ${element.textContent}`);
    }
    if (document.documentElement.scrollWidth > innerWidth) errors.push('horizontal overflow');
    return errors;
  });
  expect(problems).toEqual([]);
});

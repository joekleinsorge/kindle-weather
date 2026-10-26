const { test, expect } = require('@playwright/test');

for (const scenario of ['paperwhite1', 'paperwhite1&wide-time']) {
  test(`2012 Paperwhite tide text stays clear of chart: ${scenario}`, async ({ browser }, testInfo) => {
    const landscape = testInfo.project.name.includes('landscape');
    const context = await browser.newContext({ javaScriptEnabled: false, viewport: testInfo.project.use.viewport });
    const page = await context.newPage();
    const failedAssets = [];
    page.on('response', response => {
      if (['stylesheet', 'font'].includes(response.request().resourceType()) && response.status() !== 200)
        failedAssets.push(`${response.status()} ${response.url()}`);
    });
    const response = await page.goto(`/?${landscape ? 'h&' : ''}${scenario}`);
    expect(response.status()).toBe(200);
    await page.evaluate(() => document.fonts.ready);
    const problems = await page.evaluate(() => {
      const errors = [];
      const time = document.querySelector('#tide-band .band-big');
      const range = document.createRange();
      range.selectNodeContents(time);
      const text = range.getBoundingClientRect();
      const side = document.querySelector('#tide-band .band-side').getBoundingClientRect();
      if (text.right > side.left - 4) errors.push('tide time touches chart/caption column');
      const spark = document.querySelector('.tide-spark').getBoundingClientRect();
      const caption = document.querySelector('#tide-band .band-caption').getBoundingClientRect();
      if (spark.bottom > caption.top + 1) errors.push('tide chart overlaps caption');
      for (const element of document.querySelectorAll('#tide-band .band-main, #tide-band .band-side, #tide-band .band-caption, .rain-bars')) {
        if (element.scrollWidth > element.clientWidth + 1) errors.push(`horizontal overflow in ${element.className}`);
      }
      if (document.documentElement.scrollWidth > innerWidth) errors.push('page overflows horizontally');
      if (document.querySelectorAll('script').length) errors.push('dashboard requires JavaScript');
      return errors;
    });
    expect(problems).toEqual([]);
    expect(failedAssets).toEqual([]);
    await expect(page.locator('#tide-band .band-big')).toContainText(scenario.includes('wide-time') ? '12:59' : '7:32');
    await page.screenshot({ path: testInfo.outputPath('paperwhite1.png'), fullPage: false });
    await context.close();
  });
}

for (const scenario of ['', 'long', 'minimal', 'possible', 'stale']) {
  test(`Kindle layout fits: ${scenario || 'photo summary'}`, async ({ page }, testInfo) => {
    const landscape = testInfo.project.name.includes('landscape');
    await page.goto(`/?${landscape ? 'h&' : ''}${scenario}`);
    await page.evaluate(() => document.fonts.ready);
    const problems = await page.evaluate(() => {
      const errors = [];
      // Bands may sit side by side in landscape, so check every pair of rows.
      const rows = ['#masthead', '#current-weather', '#day-ahead', '#rain-band', '#tide-band', '#beach-status', '#insights', '#footer']
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
      const bottom = document.querySelector('#overview').getBoundingClientRect().bottom;
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

test('overview context and navigation fit on screen', async ({ page }, testInfo) => {
  const landscape = testInfo.project.name.includes('landscape');
  await page.goto(`/?${landscape ? 'h&' : ''}`);
  await expect(page.locator('#condition')).toHaveText('Partly cloudy');
  await expect(page.locator('#day-ahead .forecast-hour')).toHaveCount(4);
  await expect(page.locator('#overview nav a[aria-current="page"]')).toHaveText('Overview');
  expect(await page.locator('#insights').evaluate(el => el.getBoundingClientRect().bottom))
    .toBeLessThanOrEqual(await page.locator('#current-weather').evaluate(el => el.getBoundingClientRect().top));
  await expect(page.locator('#daily-range')).toHaveText('High 91° · Low 76°');
  await expect(page.locator('.weather-updated')).toHaveText('Updated Thu 8:20 AM');
  const box = await page.locator('#overview').boundingBox();
  expect(box.y + box.height).toBeLessThanOrEqual(testInfo.project.use.viewport.height);
  await expect(page.locator('#rain-band .chart-axis')).toContainText('8a');
  await expect(page.locator('#tide-band .chart-axis')).toContainText('12p');
});

test('large panels open details without JavaScript', async ({ browser }, testInfo) => {
  const landscape = testInfo.project.name.includes('landscape');
  const context = await browser.newContext({ javaScriptEnabled: false, viewport: testInfo.project.use.viewport });
  const page = await context.newPage();
  for (const [panel, section] of [['rain-band', 'rain'], ['tide-band', 'tides'], ['beach-status', 'surf']]) {
    await page.goto(`/?${landscape ? 'h&' : ''}`);
    await page.locator(`#${panel} .band-link`).click();
    await expect(page).toHaveURL(new RegExp(`section=${section}#${section}$`));
    if (landscape) expect(new URL(page.url()).searchParams.has('h')).toBeTruthy();
  }
  await context.close();
});

test('solid black marks only likely rain and cached weather', async ({ page }, testInfo) => {
  const prefix = testInfo.project.name.includes('landscape') ? 'h&' : '';
  await page.goto(`/?${prefix}`);
  await expect(page.locator('#rain-band')).toHaveClass(/inverse/);
  await expect(page.locator('#day-ahead .forecast-condition')).toHaveCount(0);
  await expect(page.locator('#surf tr.best')).toHaveCount(2);
  await page.goto(`/?${prefix}paperwhite1`);
  await expect(page.locator('#rain-band')).toHaveClass(/calm/);
  await page.goto(`/?${prefix}stale`);
  await expect(page.locator('.data-status')).toHaveText('Weather from Thu 4:05 AM · updates unavailable');
  await expect(page.locator('#current-weather')).toHaveClass(/stale/);
  await expect(page.locator('#feels')).toHaveText('as of 4:05 AM');
});

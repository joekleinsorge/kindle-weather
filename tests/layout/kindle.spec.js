const { test, expect } = require('@playwright/test');

for (const scenario of ['', 'long', 'minimal']) {
  test(`Kindle layout fits: ${scenario || 'photo summary'}`, async ({ page }, testInfo) => {
    const landscape = testInfo.project.name.includes('landscape');
    await page.goto(`/?${landscape ? 'h&' : ''}${scenario}`);
    await page.evaluate(() => document.fonts.ready);
    const problems = await page.evaluate(() => {
      const errors = [];
      const rows = ['#current-weather', '#description', '#insights', '.forecast', '.tide-section', '#footer'];
      let bottom = 0;
      for (const selector of rows) {
        const element = document.querySelector(selector);
        const box = element.getBoundingClientRect();
        if (box.top < bottom - 1) errors.push(`${selector} overlaps previous row`);
        bottom = box.bottom;
      }
      if (bottom > innerHeight) errors.push(`footer ends at ${bottom}, viewport ends at ${innerHeight}`);
      if (document.documentElement.scrollWidth > innerWidth) errors.push('horizontal overflow');
      // Check individual labels as well as row boxes: clipped text can fit inside a broken row.
      for (const element of document.querySelectorAll('.col, #description, .insight, #sun, #launches')) {
        if (element.scrollWidth > element.clientWidth + 1 || element.scrollHeight > element.clientHeight + 1)
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

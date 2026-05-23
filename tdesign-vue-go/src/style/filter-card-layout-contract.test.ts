import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const readFixture = (relativePath: string) =>
  readFileSync(fileURLToPath(new URL(relativePath, import.meta.url)), 'utf8');

describe('console filter card layout contract', () => {
  const sharedStyleFiles = ['operation-list.less', 'system-management.less'];

  it.each(sharedStyleFiles)('uses the stacked filter surface with a separated action bar in %s', (styleFile) => {
    const source = readFixture(`./${styleFile}`);

    expect(source).not.toContain('grid-template-columns: max-content minmax(280px, 1fr) max-content;');
    expect(source).not.toContain("grid-template-areas: 'head actions' 'form actions';");
    expect(source).toContain("grid-template-areas: 'head' 'form' 'actions';");
    expect(source).toContain('grid-area: head;');
    expect(source).toContain('grid-area: form;');
    expect(source).toContain('grid-area: actions;');
    expect(source).toContain('border-top: 1px solid #e6edf7;');
    expect(source).toContain('background: linear-gradient(90deg, #f8fbff, #fbfefe);');
    expect(source).toContain('grid-template-columns: repeat(auto-fit, minmax(min(240px, 100%), 320px));');
    expect(source).toContain('.filter-card__actions > .t-space:first-child');
    expect(source).toContain('margin-left: auto;');
    expect(source).toContain('> .filter-card > .t-loading__parent > .t-card__body');
  });

  it('keeps standalone action buttons visible in the operation list toolbar', () => {
    const source = readFixture('./operation-list.less');

    expect(source).not.toMatch(/\.filter-card__actions\s*>\s*\.t-button\s*\{\s*display:\s*none;/);
  });

  it('brings the monitor job page into the shared operation-list shell', () => {
    const source = readFixture('../pages/monitor/job/index.vue');

    expect(source).toContain('class="job-page ops-list-page"');
  });
});

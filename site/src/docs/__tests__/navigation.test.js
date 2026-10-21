import { describe, it, expect } from 'vitest';
import { PAGE_ORDER, navGroups, titleFromRaw } from '../loader.js';

// The overview page (docs/README.md), the contributing index
// (docs/contributing/README.md) and the sidebar (PAGE_ORDER) must list the same
// pages, in the same order, under each page's own H1 title.
const files = import.meta.glob('../../../../docs/**/*.md', {
  query: '?raw', import: 'default', eager: true,
});
const raw = Object.fromEntries(
  Object.entries(files).map(([path, text]) => [path.replace(/^.*\/docs\//, '').replace(/\.md$/, ''), text]),
);

const SECTIONS = {
  Reference: 'reference',
  Drivers: 'drivers',
  Orchestrators: 'orchestrators',
  'How-to': 'how-to',
  Contributing: 'contributing',
};

function section(markdown, heading) {
  const start = markdown.indexOf(`\n## ${heading}\n`);
  if (start === -1) throw new Error(`missing section: ## ${heading}`);
  const rest = markdown.slice(start + heading.length + 5);
  const end = rest.search(/^## /m);
  return end === -1 ? rest : rest.slice(0, end);
}

// Markdown links to <prefix><page>.md, as { rel, text } in document order.
// Only bare `<prefix><page>.md)` targets match: a link with an anchor
// (`page.md#x`) or a `./` prefix is not counted, so the list comparison fails.
// That is by design: the overview and the indexes link to pages, not sections.
function pageLinks(markdown, prefix, folder) {
  const pattern = new RegExp(`\\[([^\\]]+)\\]\\(${prefix.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}([\\w-]+)\\.md\\)`, 'g');
  return [...markdown.matchAll(pattern)].map(([, text, page]) => ({ rel: `${folder}/${page}`, text }));
}

const ordered = (folder) => PAGE_ORDER.filter((rel) => rel.startsWith(`${folder}/`));
const onDisk = (folder) => Object.keys(raw)
  .filter((rel) => rel.startsWith(`${folder}/`) && rel !== `${folder}/README`)
  .sort();
const expected = (folder) => ordered(folder).map((rel) => ({ rel, text: titleFromRaw(raw[rel], rel) }));

describe('docs navigation', () => {
  for (const [heading, folder] of Object.entries(SECTIONS)) {
    describe(heading, () => {
      it('lists every page in PAGE_ORDER', () => {
        expect([...ordered(folder)].sort()).toEqual(onDisk(folder));
      });

      it('shows the sidebar in PAGE_ORDER with the pages\' H1 titles', () => {
        const group = navGroups.find((g) => g.name === heading);
        expect(group.pages.map((p) => ({ rel: p.rel, text: p.title }))).toEqual(expected(folder));
      });

      it('lists the same pages, order and titles on the overview page', () => {
        const links = pageLinks(section(raw.README, heading), `${folder}/`, folder);
        expect(links).toEqual(expected(folder));
      });
    });
  }

  // The sidebar column is 232px wide; a longer H1 wraps onto a second line.
  it('keeps every sidebar title within 28 characters', () => {
    const long = navGroups.flatMap((g) => g.pages).filter((p) => p.title.length > 28);
    expect(long.map((p) => `${p.rel}: ${p.title}`)).toEqual([]);
  });

  it('lists the same pages, order and titles in the contributing index', () => {
    const table = raw['contributing/README']
      .split('\n')
      .filter((line) => line.startsWith('| ['))
      .join('\n');
    expect(pageLinks(table, '', 'contributing')).toEqual(expected('contributing'));
  });
});

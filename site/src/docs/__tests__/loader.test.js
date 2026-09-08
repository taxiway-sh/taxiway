import { describe, it, expect } from 'vitest';
import { docs, navGroups, resolveDocLink, titleFromRaw } from '../loader.js';

describe('docs loader', () => {
  it('maps README to /docs and mirrors the folder path in the URL', () => {
    for (const doc of docs) {
      expect(doc.route).toBe(doc.rel === 'README' ? '/docs' : `/docs/${doc.rel}`);
    }
  });

  it('has no duplicate routes', () => {
    const seen = new Set(docs.map(d => d.route));
    expect(seen.size).toBe(docs.length);
  });

  it('derives a title from the first H1', () => {
    expect(titleFromRaw('Intro\n\n# First title\n\n# Second title', 'Fallback')).toBe('First title');
    expect(titleFromRaw('No heading', 'Fallback')).toBe('Fallback');
  });

  it('groups pages and keeps only non-empty groups', () => {
    expect(navGroups.every(g => g.pages.length > 0)).toBe(true);
    for (const group of navGroups) {
      expect(group.pages.every(page => page.group === group.name)).toBe(true);
    }
  });

  it('exposes the index first as a headingless "Overview" link', () => {
    // Index group has an empty name so it renders as a lone top-level link.
    expect(navGroups[0].name).toBe('');
    expect(navGroups[0].pages).toHaveLength(1);
    const index = docs.find(d => d.route === '/docs');
    expect(index.title).toBe('Overview');
    expect(navGroups[0].pages[0].route).toBe('/docs');
  });

  it('keeps the contributing directory index out of the site pages', () => {
    expect(docs.some(d => d.rel === 'contributing/README')).toBe(false);
    expect(resolveDocLink('README', 'contributing/README.md')).toEqual({
      kind: 'internal', to: '/docs#contributing',
    });
  });

});

describe('resolveDocLink', () => {
  it('marks http(s) links as external', () => {
    expect(resolveDocLink('how-to/gateway', 'https://example.com')).toEqual({
      kind: 'external', href: 'https://example.com',
    });
  });

  it('keeps bare anchors as same-page anchors', () => {
    expect(resolveDocLink('reference/concepts', '#labs')).toEqual({
      kind: 'anchor', href: '#labs',
    });
  });

  it('resolves a sibling .md link to its internal route', () => {
    expect(resolveDocLink('how-to/gateway', 'observability.md')).toEqual({
      kind: 'internal', to: '/docs/how-to/observability',
    });
  });

  it('resolves a .md link from the README index', () => {
    expect(resolveDocLink('README', 'reference/concepts.md')).toEqual({
      kind: 'internal', to: '/docs/reference/concepts',
    });
  });

  it('resolves parent-relative links and preserves the anchor', () => {
    expect(resolveDocLink('reference/architecture', '../README.md#drivers')).toEqual({
      kind: 'internal', to: '/docs#drivers',
    });
    expect(resolveDocLink('how-to/recordings', '../drivers/docker.md')).toEqual({
      kind: 'internal', to: '/docs/drivers/docker',
    });
  });

  it('resolves a same-file .md link with an anchor', () => {
    expect(resolveDocLink('contributing/development', 'development.md#commit-messages')).toEqual({
      kind: 'internal', to: '/docs/contributing/development#commit-messages',
    });
  });

  it('leaves non-md relative links as external', () => {
    expect(resolveDocLink('reference/concepts', '../LICENSE')).toEqual({
      kind: 'external', href: '../LICENSE',
    });
  });
});

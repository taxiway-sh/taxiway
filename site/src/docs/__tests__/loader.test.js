import { describe, it, expect } from 'vitest';
import { docs, navGroups, resolveDocLink } from '../loader.js';

describe('docs loader', () => {
  it('creates one entry per docs markdown file', () => {
    // 16 markdown files exist under docs/ at time of writing.
    expect(docs.length).toBe(16);
  });

  it('maps README to /docs and mirrors the folder path in the URL', () => {
    const routes = Object.fromEntries(docs.map(d => [d.rel, d.route]));
    expect(routes['README']).toBe('/docs');
    expect(routes['reference/concepts']).toBe('/docs/reference/concepts');
    expect(routes['how-to/gateway']).toBe('/docs/how-to/gateway');
    expect(routes['reference/installation']).toBe('/docs/reference/installation');
    expect(routes['drivers/lima']).toBe('/docs/drivers/lima');
    expect(routes['contributing/development']).toBe('/docs/contributing/development');
  });

  it('has no duplicate routes', () => {
    const seen = new Set(docs.map(d => d.route));
    expect(seen.size).toBe(docs.length);
  });

  it('derives a title from the first H1', () => {
    const concepts = docs.find(d => d.route === '/docs/reference/concepts');
    expect(concepts.title.length).toBeGreaterThan(0);
    expect(concepts.raw).toContain('#');
  });

  it('groups pages and keeps only non-empty groups', () => {
    const names = navGroups.map(g => g.name);
    expect(names).toContain('Reference');
    expect(names).toContain('How-to');
    expect(navGroups.every(g => g.pages.length > 0)).toBe(true);
  });

  it('orders how-to and contributing to match the overview page', () => {
    const titlesOf = (name) => navGroups.find(g => g.name === name).pages.map(p => p.title);
    expect(titlesOf('How-to')).toEqual(['Gateway', 'Observability', 'Recordings']);
    expect(titlesOf('Contributing')).toEqual([
      'Development', 'Testing', 'Issues', 'Release', 'Installation qualification',
    ]);
    expect(titlesOf('Orchestrators')).toEqual(['Claude Code', 'Codex', 'Gas Town']);
  });

  it('exposes the index first as a headingless "Overview" link', () => {
    // Index group has an empty name so it renders as a lone top-level link.
    expect(navGroups[0].name).toBe('');
    expect(navGroups[0].pages).toHaveLength(1);
    const index = docs.find(d => d.route === '/docs');
    expect(index.title).toBe('Overview');
    expect(navGroups[0].pages[0].route).toBe('/docs');
  });

  it('places Installation between Concepts and CLI Usage in Reference', () => {
    const pages = navGroups.find(g => g.name === 'Reference').pages;
    expect(pages.map(p => p.rel)).toEqual([
      'reference/concepts', 'reference/installation', 'reference/commands',
      'reference/configuration', 'reference/architecture',
    ]);
  });

  it('places installation qualification immediately after Release', () => {
    const pages = navGroups.find(g => g.name === 'Contributing').pages;
    const releaseIndex = pages.findIndex(p => p.rel === 'contributing/release');
    expect(releaseIndex).toBeGreaterThanOrEqual(0);
    expect(pages[releaseIndex + 1].route).toBe('/docs/contributing/installation-qualification');
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

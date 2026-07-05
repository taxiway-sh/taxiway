// Eagerly import every docs/*.md as raw text, then build the page + nav model.
// Path is relative to this file: src/docs -> repo root is ../../../.
const modules = import.meta.glob(
  '../../../docs/**/*.md',
  { query: '?raw', import: 'default', eager: true },
);

export function titleFromRaw(raw, fallback) {
  const m = raw.match(/^#\s+(.+?)\s*$/m);
  return m ? m[1] : fallback;
}

function humanize(segment) {
  return segment.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

// The index (README) has no section heading — it renders as a lone top link.
const INDEX_GROUP = '';

function groupFor(rel) {
  if (rel === 'README') return INDEX_GROUP;
  if (rel.startsWith('drivers/')) return 'Drivers';
  if (rel.startsWith('orchestrators/')) return 'Orchestrators';
  if (rel.startsWith('reference/')) return 'Reference';
  if (rel.startsWith('how-to/')) return 'How-to';
  if (rel.startsWith('contributing/')) return 'Contributing';
  throw new Error(`docs loader: unexpected doc path with no known group: ${rel}`);
}

function routeFor(rel) {
  // Folder path == URL path: /docs/<category>/<page>. The index (README) is /docs.
  return rel === 'README' ? '/docs' : `/docs/${rel}`;
}

function titleFor(rel, raw) {
  // The docs index (README, page title "Understanding Taxiway") shows as a
  // short "Overview" link in the sidebar.
  if (rel === 'README') return 'Overview';
  return titleFromRaw(raw, humanize(rel.split('/').pop()));
}

// Resolve a raw markdown link target (as authored in docs/*.md) into either an
// internal site route, a same-page anchor, or an external URL. `currentRel` is
// the rel path of the doc containing the link (e.g. "how-to/gateway").
export function resolveDocLink(currentRel, href) {
  if (/^(https?:|mailto:|tel:)/i.test(href)) return { kind: 'external', href };
  if (href.startsWith('#')) return { kind: 'anchor', href };

  const [rawPath, anchor] = href.split('#');
  if (!/\.md$/i.test(rawPath)) return { kind: 'external', href };

  const dir = currentRel.includes('/') ? currentRel.replace(/\/[^/]*$/, '') : '';
  const segments = dir ? dir.split('/') : [];
  for (const part of rawPath.replace(/\.md$/i, '').split('/')) {
    if (part === '..') segments.pop();
    else if (part !== '.' && part !== '') segments.push(part);
  }
  const to = routeFor(segments.join('/')) + (anchor ? `#${anchor}` : '');
  return { kind: 'internal', to };
}

// Explicit page order so the sidebar matches the overview page. Anything not
// listed falls back to the end, alphabetically.
const PAGE_ORDER = [
  'README',
  'reference/concepts', 'reference/commands', 'reference/configuration', 'reference/architecture',
  'drivers/lima', 'drivers/docker',
  'orchestrators/claude-code', 'orchestrators/codex', 'orchestrators/gastown',
  'how-to/gateway', 'how-to/observability', 'how-to/recordings',
  'contributing/development', 'contributing/testing', 'contributing/release',
];
const orderIndex = (rel) => {
  const i = PAGE_ORDER.indexOf(rel);
  return i === -1 ? PAGE_ORDER.length : i;
};

export const docs = Object.entries(modules)
  .map(([filePath, raw]) => {
    const rel = filePath.replace(/^.*\/docs\//, '').replace(/\.md$/, '');
    return {
      rel,
      route: routeFor(rel),
      group: groupFor(rel),
      title: titleFor(rel, raw),
      raw,
    };
  })
  .sort((a, b) => orderIndex(a.rel) - orderIndex(b.rel) || a.route.localeCompare(b.route));

const GROUP_ORDER = [INDEX_GROUP, 'Reference', 'Drivers', 'Orchestrators', 'How-to', 'Contributing'];

export const navGroups = GROUP_ORDER
  .map((name) => ({ name, pages: docs.filter((d) => d.group === name) }))
  .filter((g) => g.pages.length > 0);

// Category folder segments (reference, drivers, ...). Bare /docs/<category>
// paths have no page of their own; they redirect to the overview's #anchor.
export const categories = [
  ...new Set(docs.map((d) => d.rel.split('/')[0]).filter((seg) => seg !== 'README')),
];

import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import ReactMarkdown from 'react-markdown';
import { describe, it, expect } from 'vitest';
import { remarkPlugins, rehypePlugins } from '../DocPage.jsx';
import { docs, resolveDocLink } from '../loader.js';

// Every link in every docs/ page, walked through resolveDocLink: an internal
// link must reach an existing page route and, with an anchor, an existing
// heading id of that page; a same-page anchor must exist on the page; any other
// link, and every image, must be absolute, because the site cannot serve a
// relative non-page path. Bodies render with the site's own Markdown
// pipeline, so links and heading ids are the ones the site shows. The
// GitHub-only contributing index is walked too: GitHub renders it, and its
// links must work on both.
const files = import.meta.glob('../../../../docs/**/*.md', {
  query: '?raw', import: 'default', eager: true,
});
const sources = Object.entries(files)
  .map(([path, raw]) => ({ rel: path.replace(/^.*\/docs\//, '').replace(/\.md$/, ''), raw }))
  .sort((a, b) => a.rel.localeCompare(b.rel));

function render(raw) {
  const hrefs = [];
  const images = [];
  const components = {
    a({ href = '', children }) {
      hrefs.push(href);
      return <a href={href}>{children}</a>;
    },
    img({ src = '', alt }) {
      images.push(src);
      return <img src={src} alt={alt} />;
    },
  };
  const html = renderToStaticMarkup(
    <ReactMarkdown remarkPlugins={remarkPlugins} rehypePlugins={rehypePlugins} components={components}>
      {raw}
    </ReactMarkdown>,
  );
  const page = new DOMParser().parseFromString(html, 'text/html');
  return { hrefs, images, ids: new Set([...page.querySelectorAll('[id]')].map((el) => el.id)) };
}

const rendered = new Map(sources.map((source) => [source.rel, render(source.raw)]));
const pageIds = new Map(docs.map((doc) => [doc.route, rendered.get(doc.rel).ids]));

const ABSOLUTE = /^(https?:|mailto:|tel:)/i;

export function brokenLinks(rel, { hrefs, images = [], ids }, pages = pageIds) {
  const broken = [];
  for (const src of images) {
    if (!/^https?:/i.test(src)) broken.push(`${src}: relative image the site does not serve`);
  }
  for (const href of hrefs) {
    const link = resolveDocLink(rel, href);
    if (link.kind === 'anchor') {
      if (!ids.has(decodeURIComponent(href.slice(1)))) broken.push(`${href}: no such heading on this page`);
    } else if (link.kind === 'internal') {
      const [route, anchor, ...extra] = link.to.split('#');
      const authored = href.split('#')[1];
      if (extra.length) broken.push(`${href}: resolves to ${link.to}, with more than one #`);
      else if (!pages.has(route)) broken.push(`${href}: resolves to ${link.to}, which is not a page`);
      else if (authored && anchor !== authored) broken.push(`${href}: resolves to ${link.to}, which drops the anchor`);
      else if (anchor && !pages.get(route).has(decodeURIComponent(anchor))) {
        broken.push(`${href}: resolves to ${link.to}, which has no such heading`);
      }
    } else if (!ABSOLUTE.test(href)) {
      broken.push(`${href}: relative link to a file the site does not serve`);
    }
  }
  return broken;
}

describe('docs links', () => {
  it('walks links of every docs page', () => {
    expect(sources.length).toBeGreaterThan(docs.length);
    expect(sources.reduce((n, s) => n + rendered.get(s.rel).hrefs.length, 0)).toBeGreaterThan(100);
  });

  for (const { rel } of sources) {
    it(`resolves every link in ${rel}.md`, () => {
      expect(brokenLinks(rel, rendered.get(rel))).toEqual([]);
    });
  }

  it('reports each kind of broken link', () => {
    const pages = new Map([['/docs/how-to/gateway', new Set(['access'])], ['/docs', new Set(['contributing'])]]);
    const sample = {
      ids: new Set(['here']),
      hrefs: [
        '#here', '#gone', 'gateway.md#access', 'gateway.md#nowhere', 'missing.md',
        '../contributing/README.md', '../contributing/README.md#at-a-glance',
        '../../AGENTS.md', '../assets/splash-screen.png', 'https://example.com/x.md', 'mailto:a@example.com',
        'tel:+100',
      ],
      images: ['../assets/splash-screen.png', 'https://example.com/x.png'],
    };
    expect(brokenLinks('how-to/observability', sample, pages)).toEqual([
      '../assets/splash-screen.png: relative image the site does not serve',
      '#gone: no such heading on this page',
      'gateway.md#nowhere: resolves to /docs/how-to/gateway#nowhere, which has no such heading',
      'missing.md: resolves to /docs/how-to/missing, which is not a page',
      '../contributing/README.md#at-a-glance: resolves to /docs#contributing, which drops the anchor',
      '../../AGENTS.md: resolves to /docs/AGENTS, which is not a page',
      '../assets/splash-screen.png: relative link to a file the site does not serve',
    ]);
  });
});

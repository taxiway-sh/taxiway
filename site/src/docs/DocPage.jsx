import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import rehypeSlug from 'rehype-slug';
import { Link, useLocation } from 'react-router-dom';
import { NavBar } from '../sections/NavBar.jsx';
import { Footer } from '../sections/CtaSection.jsx';
import { DocsNav } from './DocsNav.jsx';
import { Mermaid } from './Mermaid.jsx';
import { resolveDocLink, docs, titleFromRaw } from './loader.js';
import { TWIcon } from '../icons.jsx';

// Flatten React children (raw text or highlighted spans) back to plain text.
function toText(node) {
  if (node == null) return '';
  if (typeof node === 'string') return node;
  if (Array.isArray(node)) return node.map(toText).join('');
  if (node.props && node.props.children != null) return toText(node.props.children);
  return '';
}
import 'highlight.js/styles/github-dark.css';
import './docs.css';
import '../styles/page.css';

// A fenced code block with a copy-to-clipboard button.
function CodeBlock({ children }) {
  const ref = React.useRef(null);
  const [copied, setCopied] = React.useState(false);
  const onCopy = () => {
    const text = ref.current?.innerText ?? '';
    navigator.clipboard?.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1400);
  };
  return (
    <div className="tw-codeblock">
      <div className="tw-codeblock-bar">
        <span className="tw-codeblock-dots" aria-hidden="true"><i /><i /><i /></span>
        <button type="button" className="tw-codeblock-copy" onClick={onCopy}
          aria-label={copied ? 'Copied' : 'Copy code'}>
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre ref={ref}>{children}</pre>
    </div>
  );
}

// A heading with an anchor link icon on the right. The page title (h1) links
// to the page itself (its clean route, no hash); section headings (h2/h3) link
// to their own #anchor. rehypeSlug puts the slug id on node.properties.id.
function headingWithAnchor(Tag, { iconSize = 15, pageRoute } = {}) {
  return function Heading({ node, children }) {
    const id = node?.properties?.id;
    const icon = <TWIcon name="link" size={iconSize} />;
    const anchor = pageRoute ? (
      <Link to={pageRoute} className="tw-heading-anchor" aria-label="Link to this page">{icon}</Link>
    ) : id ? (
      <a href={`#${id}`} className="tw-heading-anchor" aria-label="Link to this section">{icon}</a>
    ) : null;
    return <Tag id={id} className="tw-heading">{children}{anchor}</Tag>;
  };
}

// Rewrite in-body markdown links: repo-relative .md links become internal
// routes, external links open in a new tab, bare anchors stay same-page.
// Fenced code blocks get a copy button; headings get an anchor link.
function markdownComponents(currentRel, pageRoute) {
  return {
    a({ href = '', children, node, ...props }) {
      const link = resolveDocLink(currentRel, href);
      if (link.kind === 'internal') return <Link to={link.to}>{children}</Link>;
      if (link.kind === 'anchor') return <a href={link.href}>{children}</a>;
      return <a href={link.href} target="_blank" rel="noreferrer">{children}</a>;
    },
    pre({ children }) {
      const child = React.Children.toArray(children)[0];
      const cls = (child && child.props && child.props.className) || '';
      if (/\blanguage-mermaid\b/.test(cls)) {
        return <Mermaid code={toText(child.props.children).replace(/\n$/, '')} />;
      }
      return <CodeBlock>{children}</CodeBlock>;
    },
    table({ children }) {
      // Wrap tables in a bordered, rounded card (per the DS) that also scrolls
      // wide tables horizontally without overflowing the page.
      return <div className="tw-tablewrap"><table>{children}</table></div>;
    },
    h1: headingWithAnchor('h1', { iconSize: 19, pageRoute }),
    h2: headingWithAnchor('h2'),
    h3: headingWithAnchor('h3'),
  };
}

// Previous / next pager built from the ordered docs list (same order as the
// sidebar). Each card shows the direction, the target's section, and title.
// No wrap: the first page has no Previous, the last no Next.
function DocPager({ current }) {
  const i = docs.findIndex((d) => d.route === current);
  const prev = i > 0 ? docs[i - 1] : null;
  const next = i >= 0 && i < docs.length - 1 ? docs[i + 1] : null;
  if (!prev && !next) return null;
  const section = (d) => d.group || 'Overview';
  // Use the page's real H1 for the pager title (the index's short nav label is
  // "Overview", but its heading is "Understanding Taxiway").
  const heading = (d) => titleFromRaw(d.raw, d.title);
  const Card = ({ doc: d, dir }) => (
    <Link to={d.route} className={`tw-pager-link tw-pager-${dir === 'Previous' ? 'prev' : 'next'}`}>
      <span className="tw-pager-label">
        <span className="tw-pager-dir">{dir}</span>
        <span className="tw-pager-sep">·</span>
        <span className="tw-pager-cat">{section(d)}</span>
      </span>
      <span className="tw-pager-title">{heading(d)}</span>
    </Link>
  );
  return (
    <nav className="tw-docs-pager" aria-label="Pagination">
      {prev ? <Card doc={prev} dir="Previous" /> : <span />}
      {next ? <Card doc={next} dir="Next" /> : <span />}
    </nav>
  );
}

// "On this page" TOC, built client-side from the rendered H2 headings (their
// ids come from rehypeSlug, so the anchors always match). It's a nav aid, so it
// only appears after hydration and simply vanishes on narrow layouts.
function TableOfContents({ scope }) {
  const [items, setItems] = React.useState([]);
  const [active, setActive] = React.useState('');

  React.useEffect(() => {
    const hs = Array.from(document.querySelectorAll('.tw-prose h2[id]'));
    setItems(hs.map((h) => {
      const clone = h.cloneNode(true);
      clone.querySelectorAll('.tw-heading-anchor').forEach((a) => a.remove());
      return { id: h.id, text: clone.textContent.trim() };
    }));
  }, [scope]);

  React.useEffect(() => {
    if (!items.length) return undefined;
    let raf = 0;
    const update = () => {
      raf = 0;
      const line = window.scrollY + 120;
      let cur = items[0].id;
      for (const it of items) {
        const el = document.getElementById(it.id);
        if (el && el.getBoundingClientRect().top + window.scrollY <= line) cur = it.id;
      }
      setActive(cur);
    };
    const onScroll = () => { if (!raf) raf = requestAnimationFrame(update); };
    window.addEventListener('scroll', onScroll, { passive: true });
    update();
    return () => { window.removeEventListener('scroll', onScroll); if (raf) cancelAnimationFrame(raf); };
  }, [items]);

  return (
    <aside className="tw-docs-toc" aria-label="On this page">
      {items.length > 0 && (
        <nav>
          <p className="tw-docs-toc-h">On this page</p>
          {items.map((it) => (
            <a key={it.id} href={`#${it.id}`} className={it.id === active ? 'is-active' : undefined}>{it.text}</a>
          ))}
        </nav>
      )}
    </aside>
  );
}

export function DocPage({ doc }) {
  const { pathname, hash } = useLocation();
  const components = React.useMemo(() => markdownComponents(doc.rel, doc.route), [doc.rel, doc.route]);
  // On mobile the sidebar is an off-canvas drawer; closed by default.
  const [navOpen, setNavOpen] = React.useState(false);

  // Scroll to the anchored section after render; reset to top when the page
  // changes without an anchor. rehypeSlug gives every heading a matching id.
  React.useEffect(() => {
    if (hash) {
      const el = document.getElementById(decodeURIComponent(hash.slice(1)));
      if (el) { el.scrollIntoView(); return; }
    }
    window.scrollTo(0, 0);
  }, [pathname, hash]);

  // Close the drawer when navigating to another page and on Escape.
  React.useEffect(() => { setNavOpen(false); }, [pathname]);
  React.useEffect(() => {
    if (!navOpen) return undefined;
    const onKey = (e) => { if (e.key === 'Escape') setNavOpen(false); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [navOpen]);

  return (
    <div className="tw-page-bg">
      <NavBar />
      <div className="tw-docs-shell">
        <DocsNav current={doc.route} currentGroup={doc.group} open={navOpen} onClose={() => setNavOpen(false)} />
        {navOpen && <div className="tw-docs-backdrop" onClick={() => setNavOpen(false)} />}
        <button type="button" className="tw-docs-drawer-toggle"
          aria-label={navOpen ? 'Hide navigation' : 'Show navigation'}
          aria-expanded={navOpen} aria-controls="tw-docs-nav"
          onClick={() => setNavOpen(o => !o)}>
          <TWIcon name={navOpen ? 'close' : 'panel'} size={22} />
        </button>
        <main className="tw-docs-main">
          <article className="tw-prose">
            {/* Section label above the title; the index (empty group) reads "Overview". */}
            <span className="tw-eyebrow" style={{ display: 'block', marginBottom: 'var(--space-3)' }}>{doc.group || 'Overview'}</span>
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              rehypePlugins={[rehypeSlug, [rehypeHighlight, { ignoreMissing: true }]]}
              components={components}
            >
              {doc.raw}
            </ReactMarkdown>
            <DocPager current={doc.route} />
          </article>
        </main>
        <TableOfContents scope={doc.route} />
      </div>
      <Footer />
    </div>
  );
}

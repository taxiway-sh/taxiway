import React from 'react';

// Mermaid is heavy and browser-only, so load it lazily on the client and render
// the diagram after hydration (the prerendered HTML ships an empty placeholder).
let mermaidPromise;
function loadMermaid() {
  if (!mermaidPromise) {
    mermaidPromise = import('mermaid').then(({ default: mermaid }) => mermaid);
  }
  return mermaidPromise;
}

// Resolve the diagram palette from the active --tw-* tokens at render time, so
// it tracks the current theme (cream nodes on light, deep-slate on dark).
function themeVars() {
  const css = getComputedStyle(document.documentElement);
  const v = (name, fallback) => (css.getPropertyValue(name) || fallback).trim();
  return {
    fontSize: '14px',
    primaryColor: v('--tw-surface', '#FDF2D6'),        // node fill
    primaryBorderColor: v('--tw-accent-2', '#1F7C79'), // node border
    primaryTextColor: v('--tw-text', '#23303A'),       // node text
    lineColor: v('--tw-muted', '#5F6156'),             // edges
    textColor: v('--tw-text-2', '#3C4852'),
    clusterBkg: v('--tw-surface-2', '#F1DEB0'),        // subgraph fill
    clusterBorder: v('--tw-border', '#E4CF9C'),        // subgraph border
    titleColor: v('--tw-accent-2', '#1F7C79'),         // subgraph title
    edgeLabelBackground: v('--tw-surface', '#FDF2D6'),
  };
}

let counter = 0;

export function Mermaid({ code }) {
  const ref = React.useRef(null);
  const [failed, setFailed] = React.useState(false);
  // Track the active theme so the diagram re-renders (with new colors) on flip.
  const [theme, setTheme] = React.useState(() =>
    typeof document !== 'undefined' ? document.documentElement.dataset.theme || 'light' : 'light');

  React.useEffect(() => {
    const el = document.documentElement;
    const obs = new MutationObserver(() => setTheme(el.dataset.theme || 'light'));
    obs.observe(el, { attributes: true, attributeFilter: ['data-theme'] });
    return () => obs.disconnect();
  }, []);

  React.useEffect(() => {
    let cancelled = false;
    setFailed(false);
    loadMermaid()
      .then((mermaid) => {
        // Re-apply the current theme's colors before each render.
        mermaid.initialize({
          startOnLoad: false,
          securityLevel: 'strict',
          theme: 'base',
          fontFamily: "'JetBrains Mono', ui-monospace, monospace",
          themeVariables: themeVars(),
        });
        return mermaid.render(`tw-mermaid-${counter++}`, code);
      })
      .then(({ svg }) => { if (!cancelled && ref.current) ref.current.innerHTML = svg; })
      .catch(() => { if (!cancelled) setFailed(true); });
    return () => { cancelled = true; };
  }, [code, theme]);

  if (failed) {
    return <pre className="tw-mermaid-fallback"><code>{code}</code></pre>;
  }
  // The SVG is decorative for assistive tech; the source (which describes the
  // flow) is exposed as a visually-hidden accessible description.
  return (
    <figure className="tw-mermaid">
      <div ref={ref} aria-hidden="true" />
      <figcaption className="sr-only">Diagram source: {code}</figcaption>
    </figure>
  );
}

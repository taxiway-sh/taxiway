/* TaxiWay landing — Inside every lab, reproduced from the design system
   landing kit: amber-tinted icon tiles on themed cards. Cards stay navigable
   to their docs, but render identically to the DS (no hover lift). */
import React from 'react';
import { Link } from 'react-router-dom';
import { TWIcon } from '../icons.jsx';

const mono = { fontFamily: 'var(--font-mono)', fontSize: '0.9em', color: 'var(--tw-accent-ink)' };

const features = [
  { icon: 'box', title: 'Isolated Sandboxes', to: '/docs#drivers',
    body: <>Run each lab in an isolated sandbox backed by Lima or Docker. Taxiway keeps the workspace, runtime, tools, and sessions contained.</> },
  { icon: 'rotate', title: 'Disposable Labs', to: '/docs/reference/concepts',
    body: <>Create repeatable labs that can be stopped, reset, or removed without hand-cleaning state. Failed runs can start again from a clean baseline.</> },
  { icon: 'network', title: 'Multi Orchestrators', to: '/docs#orchestrators',
    body: <>Use the same lab contract across Claude Code, Codex, and Gas Town. Setup, authentication, startup, and verification stay consistent.</> },
  { icon: 'route', title: 'Gateway', to: '/docs/how-to/gateway',
    body: <>A shared Caddy proxy and per-lab LiteLLM sidecars route model traffic. Lab commands start it automatically.</> },
  { icon: 'activity', title: 'Observability', to: '/docs/how-to/observability',
    body: <>Optional Langfuse trace storage. Start it with <code style={mono}>taxiway observe up</code> and open it through the proxy.</> },
  { icon: 'video', title: 'Recordings', to: '/docs/how-to/recordings',
    body: <>Record the session with asciinema and tmux, replay in the browser player, and analyze runs.</> },
];

function FeatureGrid() {
  return (
    <section id="features" style={{ scrollMarginTop: 'var(--nav-height)' }}>
      <div style={{ maxWidth: 'var(--container-wide)', margin: '0 auto', padding: 'var(--space-8) var(--space-5)' }}>
        <div style={{ maxWidth: 640, margin: '0 0 40px' }}>
          <p className="tw-eyebrow">Inside every lab</p>
          <h2 style={{ fontSize: 34, marginTop: 12 }}>Everything a lab lines up for you</h2>
        </div>
        <div className="tw-feat-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 20 }}>
          {features.map(f => (
            <Link key={f.title} to={f.to} className="tw-feat-card" style={{
              textDecoration: 'none', display: 'flex', flexDirection: 'column', gap: 12,
              background: 'var(--tw-surface)', border: '1px solid var(--tw-border)',
              borderRadius: 'var(--tw-radius)', padding: 24, color: 'inherit',
            }}>
              <span style={{
                width: 44, height: 44, borderRadius: 10,
                display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
                background: 'color-mix(in srgb, var(--tw-accent) 13%, transparent)',
                border: '1px solid color-mix(in srgb, var(--tw-accent) 32%, transparent)',
                color: 'var(--tw-accent-ink)',
              }}>
                <TWIcon name={f.icon} size={22} stroke={2.1} color="currentColor" />
              </span>
              <h3 style={{ fontSize: 18 }}>{f.title}</h3>
              <p style={{ fontSize: 14, color: 'var(--tw-muted)', lineHeight: 1.55 }}>{f.body}</p>
            </Link>
          ))}
        </div>
      </div>
    </section>
  );
}

export { FeatureGrid };

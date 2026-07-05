/* Taxiway landing — closing CTA + footer */
import React from 'react';
import { Beacon } from '../components/core';
import { InstallPill } from '../components/InstallPill.jsx';
import { TWIcon } from '../icons.jsx';
import { repoUrl, docUrl } from '../config.js';
import { Link } from 'react-router-dom';

// Closing CTA, reproduced from the design system landing kit: a compact
// horizontal dark console card — install pill + amber docs, right of the pitch.
function CtaSection() {
  return (
    <section style={{ maxWidth: 'var(--container-wide)', margin: '0 auto', padding: 'var(--space-7) var(--space-5)' }}>
      <div className="tw-cta-card" style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        gap: 28, flexWrap: 'wrap',
        background: 'var(--tw-cta-bg)', border: '1px solid var(--tw-border)',
        borderRadius: 'calc(var(--tw-radius) + 6px)', padding: '26px 32px', boxShadow: 'var(--tw-shadow-lg)',
      }}>
        <div style={{ flex: '1 1 0', minWidth: 220 }}>
          <h2 style={{ fontSize: 24, color: 'var(--tw-cta-text)' }}>Spin up your first lab</h2>
          <p style={{ color: 'var(--tw-cta-muted)', fontSize: 14, marginTop: 6 }}>
            Everything your orchestrator needs, lined up in one command.
          </p>
        </div>
        <div className="tw-cta-actions" style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
          <InstallPill />
          <Link to="/docs" style={{
            display: 'inline-flex', alignItems: 'center', gap: 8, height: 42, padding: '0 18px',
            fontFamily: 'var(--font-display)', fontSize: 14, fontWeight: 600, borderRadius: 'var(--tw-radius)',
            background: 'var(--tw-accent)', color: 'var(--tw-accent-text)', border: '1px solid var(--tw-accent)',
            textDecoration: 'none', whiteSpace: 'nowrap',
          }}>
            Read the docs <TWIcon name="arrow" size={15} stroke={2} color="currentColor" />
          </Link>
        </div>
      </div>
    </section>
  );
}

function Footer() {
  const cols = [
    { h: 'Orchestrators', links: [
      { label: 'Claude Code', to: '/docs/orchestrators/claude-code' },
      { label: 'Codex', to: '/docs/orchestrators/codex' },
      { label: 'Gas Town', to: '/docs/orchestrators/gastown' },
    ]},
    { h: 'Documentation', links: [
      { label: 'Overview', to: '/docs' },
      { label: 'Concepts', to: '/docs/reference/concepts' },
      { label: 'CLI Usage', to: '/docs/reference/commands' },
    ]},
    { h: 'Project', links: [
      { label: 'GitHub', href: repoUrl },
      { label: 'License', href: docUrl('LICENSE') },
    ]},
  ];
  return (
    <footer style={{ borderTop: '1px solid var(--tw-border)' }}>
      <div style={{ maxWidth: 'var(--container-wide)', margin: '0 auto', padding: 'var(--space-8) var(--space-5) var(--space-6)' }}>
        <div style={{ display: 'grid', gridTemplateColumns: '1.4fr 1fr 1fr 1fr', gap: 'var(--space-6)' }} className="tw-footer-grid">
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
              <Beacon size={32} />
              <span style={{ fontFamily: 'var(--font-logo)', fontWeight: 700, fontSize: 18, letterSpacing: '-0.01em', color: 'var(--tw-text)' }}>taxiway</span>
            </div>
            <p style={{ fontSize: 'var(--text-sm)', color: 'var(--tw-muted)', maxWidth: 240 }}>
              Isolated labs for running and comparing agent orchestrators.
            </p>
          </div>
          {cols.map(c => (
            <div key={c.h}>
              <h4 style={{ fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: '0.12em', textTransform: 'uppercase', color: 'var(--tw-faint)', marginBottom: 14 }}>{c.h}</h4>
              <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', gap: 10 }}>
                {c.links.map(l => (
                  <li key={l.label}>
                    {l.to ? (
                      <Link to={l.to} style={{ fontSize: 'var(--text-sm)', color: 'var(--tw-muted)', textDecoration: 'none' }}>{l.label}</Link>
                    ) : (
                      <a href={l.href} target="_blank" rel="noreferrer" style={{ fontSize: 'var(--text-sm)', color: 'var(--tw-muted)', textDecoration: 'none' }}>{l.label}</a>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
        {/* Copyright bar — divider spans only the content width (matches the DS). */}
        <div style={{
          display: 'flex', justifyContent: 'space-between', flexWrap: 'wrap', gap: 10,
          marginTop: 36, paddingTop: 20, borderTop: '1px solid var(--tw-border)',
          fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--tw-faint)',
        }}>
          <span>© 2026 Manufacture</span>
          <span>Gate to runway.</span>
        </div>
      </div>
    </footer>
  );
}

export { CtaSection, Footer };

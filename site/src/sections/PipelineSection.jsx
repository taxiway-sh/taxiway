/* TaxiWay landing — Lab lifecycle (prepare → run), reproduced from the
   design system landing kit: fluid on the page ground, amber/teal-ringed
   nodes on a teal track, one command runs the whole lifecycle. */
import React from 'react';
import { TWIcon } from '../icons.jsx';
import { Badge } from '../components/core';

// tone → node ink + glow (matches the DS TONE map)
const TONE = {
  teal: { ink: 'var(--tw-accent-2)', glow: 'var(--tw-accent-2-bright)' },
  amber: { ink: 'var(--tw-accent-ink)', glow: 'var(--tw-accent)' },
  warm: { ink: '#E07A2D', glow: '#E07A2D' },
};

const prepN = [
  ['plus', 'Create', 'Isolated lab', 'teal'],
  ['download', 'Bootstrap', 'Runtime + tools', 'teal'],
  ['cpu', 'Install', 'Orchestrator', 'amber'],
  ['check', 'Verify', 'Checks pass', 'teal'],
];
const runN = [
  ['route', 'Gateway', 'Proxy + LiteLLM', 'teal'],
  ['folder', 'Workspace', 'Repo mirror', 'teal'],
  ['shield', 'Auth', 'Credentials', 'amber'],
  ['bolt', 'Start', 'Session', 'warm'],
];

function Phase({ label, nodes }) {
  return (
    <div style={{ padding: '4px 0' }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10, marginBottom: 22,
        fontFamily: 'var(--font-mono)', fontSize: 12, letterSpacing: '0.12em',
        textTransform: 'uppercase', color: 'var(--tw-accent-2)', fontWeight: 600,
      }}>
        <span aria-hidden="true" style={{ width: 7, height: 7, borderRadius: '50%', background: 'var(--tw-accent-2)' }} />
        {label}
      </div>
      <div style={{ position: 'relative' }}>
        <div className="tw-track" aria-hidden="true" style={{
          position: 'absolute', top: 28, left: '7%', right: '7%', height: 2, opacity: 0.7,
          background: 'linear-gradient(90deg, transparent, var(--tw-accent-2) 12%, var(--tw-accent-2) 88%, transparent)',
        }} />
        <div className="tw-pipe-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, position: 'relative' }}>
          {nodes.map(([icon, name, sub, tone]) => {
            const t = TONE[tone] || TONE.amber;
            return (
              <div key={name} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', textAlign: 'center', gap: 10 }}>
                <span style={{
                  width: 56, height: 56, borderRadius: '50%',
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  background: 'var(--tw-surface)', border: `1.5px solid ${t.ink}`, color: t.ink,
                  boxShadow: `0 0 0 6px var(--tw-bg), 0 0 18px color-mix(in srgb, ${t.glow} 45%, transparent)`,
                }}>
                  <TWIcon name={icon} size={22} stroke={2} color={t.ink} />
                </span>
                <b style={{ fontFamily: 'var(--font-display)', fontSize: 15, fontWeight: 600, color: 'var(--tw-text)' }}>{name}</b>
                <span style={{ color: 'var(--tw-muted)', fontSize: 12.5 }}>{sub}</span>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

function PipelineSection() {
  return (
    <section id="lifecycle" style={{ scrollMarginTop: 'var(--nav-height)' }}>
      <div style={{ maxWidth: 'var(--container-wide)', margin: '0 auto', padding: 'var(--space-8) var(--space-5)' }}>
        <div style={{ maxWidth: 640, margin: '0 0 40px' }}>
          <p className="tw-eyebrow">Lab lifecycle</p>
          <h2 style={{ fontSize: 34, marginTop: 12 }}>Every lab runs the same lifecycle</h2>
          <p style={{ fontSize: 17, color: 'var(--tw-muted)', maxWidth: 640, margin: '20px 0 0', lineHeight: 1.55 }}>
            Two phases. <b style={{ color: 'var(--tw-text)' }}>Prepare</b> creates the lab, installs the
            orchestrator, and verifies it. <b style={{ color: 'var(--tw-text)' }}>Run</b> brings up the
            gateway and workspace, authenticates, and starts the session.
          </p>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 14, flexWrap: 'wrap', marginBottom: 34 }}>
          <span style={{
            display: 'inline-flex', alignItems: 'center', gap: 8,
            fontFamily: 'var(--font-mono)', fontSize: 14, fontWeight: 600, color: 'var(--tw-accent-ink)',
            background: 'var(--tw-surface-2)', border: '1px solid var(--tw-border)', borderRadius: 999, padding: '9px 16px',
          }}>
            <TWIcon name="terminal" size={15} stroke={2} color="var(--tw-accent-ink)" /> taxiway up
          </span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5, color: 'var(--tw-muted)' }}>
            one command runs the full lifecycle — prepare, then run
          </span>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column' }}>
          <Phase label="Prepare — build the environment" nodes={prepN} />
          <div aria-hidden="true" style={{ height: 1, background: 'var(--tw-border)', margin: '28px 0' }} />
          <Phase label="Run — wire up and start" nodes={runN} />
        </div>

        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 22 }}>
          {/* status pills — the DS Badge with colored dots, per prod */}
          <Badge tone="go" dot>idempotent phases</Badge>
          <Badge tone="signal" dot>resumable</Badge>
          <Badge tone="hold" dot>dry-run</Badge>
        </div>
      </div>
    </section>
  );
}

export { PipelineSection };

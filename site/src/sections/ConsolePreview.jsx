/* TaxiWay landing — Control tower, reproduced from the design system landing
   kit: LIVE pill · DRIVER: AUTO · $ taxiway status · four stat tiles · table. */
import React from 'react';

const rows = [
  ['taxiway-mylab', 'claude-code', 'lima', 'Running', 'go'],
  ['taxiway-codex', 'codex', 'docker', 'Running', 'go'],
  ['taxiway-gastown', 'gastown', 'lima', 'Stopped', 'off'],
  ['taxiway-bench', 'claude-code', 'docker', 'Auth', 'hold'],
  ['taxiway-demo', 'codex', 'lima', 'Running', 'go'],
];
const pc = { go: 'var(--tw-accent-2)', hold: 'var(--tw-accent-ink)', off: 'var(--tw-faint)' };
const phase = { off: '—', hold: 'auth' };
const stats = [['3', 'Labs'], ['Running', 'Proxy'], ['2', 'Gateways'], ['Running', 'Langfuse']];

function ConsolePreview() {
  return (
    <section id="control-tower" style={{ scrollMarginTop: 'var(--nav-height)' }}>
      <div style={{ maxWidth: 'var(--container-wide)', margin: '0 auto', padding: 'var(--space-8) var(--space-5)' }}>
        <div style={{ maxWidth: 640, margin: '0 0 40px' }}>
          <p className="tw-eyebrow">Control tower</p>
          <h2 style={{ fontSize: 34, marginTop: 12 }}>See every lab at a glance</h2>
        </div>

        <div style={{ background: 'var(--tw-surface)', border: '1px solid var(--tw-border)', borderRadius: 'var(--tw-radius)', overflow: 'hidden' }}>
          {/* header: title · live pill · driver switch */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '18px 22px', borderBottom: '1px solid var(--tw-border)', flexWrap: 'wrap' }}>
            <b style={{ fontFamily: 'var(--font-display)', fontSize: 17, color: 'var(--tw-text)' }}>Control tower</b>
            <span style={{
              fontFamily: 'var(--font-mono)', fontSize: 11, padding: '3px 9px', borderRadius: 999,
              display: 'inline-flex', alignItems: 'center', gap: 6,
              color: 'var(--tw-accent-2)', background: 'color-mix(in srgb, var(--tw-accent-2) 14%, transparent)',
            }}>
              <i style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--tw-accent-2)' }} />live
            </span>
            <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 10 }}>
              <span aria-hidden="true" style={{
                width: 38, height: 22, borderRadius: 999, background: 'var(--tw-accent-2)', position: 'relative', flex: '0 0 auto',
                boxShadow: '0 0 10px color-mix(in srgb, var(--tw-accent-2-bright) 45%, transparent)',
              }}>
                <span style={{ position: 'absolute', top: 2, left: 18, width: 18, height: 18, borderRadius: '50%', background: '#fff', boxShadow: '0 1px 2px rgba(0,0,0,0.3)' }} />
              </span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: '0.1em', textTransform: 'uppercase', color: 'var(--tw-muted)' }}>Driver: Auto</span>
            </div>
          </div>

          {/* status command line */}
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--tw-muted)', padding: '12px 22px', borderBottom: '1px solid var(--tw-border)' }}>$ taxiway status</div>

          {/* stat tiles */}
          <div className="tw-stat-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10, padding: '14px 22px' }}>
            {stats.map(([v, l]) => (
              <div key={l} style={{ border: '1px solid var(--tw-border)', borderRadius: 'var(--tw-radius)', padding: '9px 12px', background: 'var(--tw-surface-2)' }}>
                <b style={{ display: 'block', fontFamily: 'var(--font-display)', fontSize: 16, color: 'var(--tw-text)', lineHeight: 1.15 }}>{v}</b>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9.5, letterSpacing: '0.1em', textTransform: 'uppercase', color: 'var(--tw-muted)', marginTop: 2, display: 'block' }}>{l}</span>
              </div>
            ))}
          </div>

          {/* lab table */}
          <div style={{ overflowX: 'auto' }}>
            <table aria-label="Active labs" style={{ width: '100%', minWidth: 520, borderCollapse: 'collapse', fontFamily: 'var(--font-mono)', fontSize: 13 }}>
              <thead>
                <tr>
                  {['Lab', 'Type', 'Driver', 'Status', 'Phase'].map(h => (
                    <th key={h} style={{ textAlign: 'left', padding: '12px 22px', color: 'var(--tw-faint)', fontWeight: 500, fontSize: 11, letterSpacing: '0.08em', textTransform: 'uppercase' }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.map(([lab, type, driver, status, key]) => (
                  <tr key={lab}>
                    <td style={{ padding: '13px 22px', borderTop: '1px solid var(--tw-border)', color: 'var(--tw-accent-2)' }}>{lab}</td>
                    <td style={{ padding: '13px 22px', borderTop: '1px solid var(--tw-border)', color: 'var(--tw-text)' }}>{type}</td>
                    <td style={{ padding: '13px 22px', borderTop: '1px solid var(--tw-border)', color: 'var(--tw-muted)' }}>{driver}</td>
                    <td style={{ padding: '13px 22px', borderTop: '1px solid var(--tw-border)' }}>
                      <span style={{
                        fontFamily: 'var(--font-mono)', fontSize: 11, padding: '3px 9px', borderRadius: 999,
                        display: 'inline-flex', alignItems: 'center', gap: 6,
                        color: pc[key], background: `color-mix(in srgb, ${pc[key]} 14%, transparent)`,
                      }}>
                        <i style={{ width: 6, height: 6, borderRadius: '50%', background: pc[key] }} />{status}
                      </span>
                    </td>
                    <td style={{ padding: '13px 22px', borderTop: '1px solid var(--tw-border)', color: 'var(--tw-muted)' }}>{phase[key] || 'start'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </section>
  );
}

export { ConsolePreview };

/* Taxiway landing — hero */
import React from 'react';
import { Badge, Terminal } from '../components/core';
import { InstallPill } from '../components/InstallPill.jsx';
import { TWIcon } from '../icons.jsx';
import { Link } from 'react-router-dom';

// One quick-start row: $ command  # comment (comment stays next to the command);
// clicking copies, and a "✓ copied" marker appears on the right without removing
// the comment.
function QuickCmd({ cmd, note, copied, onCopy }) {
  return (
    <button type="button" onClick={onCopy} title="Copy command" style={{
      display: 'flex', alignItems: 'baseline', gap: 10, width: '100%',
      background: 'none', border: 'none', padding: 0, textAlign: 'left', cursor: 'pointer',
      fontFamily: 'var(--font-mono)', fontSize: 13, lineHeight: 1.9,
    }}>
      <span style={{ color: 'var(--tw-accent-2-bright)', flex: '0 0 auto' }}>$</span>
      <span style={{ color: 'var(--tw-code-text)', flex: '0 0 auto', whiteSpace: 'nowrap' }}>{cmd}</span>
      {note && (
        <span style={{ color: 'var(--tw-faint)', flex: '0 1 auto', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}># {note}</span>
      )}
      {/* plain-text marker (not an SVG) so it never grows the line height */}
      {copied && (
        <span style={{ marginLeft: 'auto', flex: '0 0 auto', color: 'var(--tw-accent-2-bright)', whiteSpace: 'nowrap' }}>✓ copied</span>
      )}
    </button>
  );
}

function Hero() {
  const Icon = TWIcon;
  const [copiedCmd, setCopiedCmd] = React.useState('');
  const copyCmd = (cmd) => {
    navigator.clipboard?.writeText(cmd);
    setCopiedCmd(cmd);
    setTimeout(() => setCopiedCmd(''), 1500);
  };
  return (
    <section className="tw-hero-section" style={{
      position: 'relative', overflow: 'hidden',
      // Full height (dvh handles mobile browser chrome). The content block uses
      // margin-block:auto — it centres when there's room and rides up to the top
      // on its own as the window gets short. A short-window media query
      // (page.css) reserves a band at the bottom so the text never lands on the
      // illustration's control desk.
      minHeight: 'calc(100dvh - var(--nav-height))', display: 'flex', flexDirection: 'column',
    }}>
      {/* full-bleed illustrated watermark: two layers that cross-fade on theme
          switch. Bottom-anchored cover — the top is cropped off-screen, no seam. */}
      <div className="tw-herobg tw-herobg-light" aria-hidden="true" />
      <div className="tw-herobg tw-herobg-dark" aria-hidden="true" />
      {/* Biased vertical rhythm: the top spacer grows less than the bottom one,
          so the content sits around the upper third (not dead-centre) on a tall
          window; both shrink as the window gets short, so the content rides up
          to the top. The bottom spacer keeps a floor = the illustration's
          control-desk band, so the text never lands on the porthole. */}
      <div aria-hidden="true" style={{ flex: '1 1 0', minHeight: '2vh' }} />
      <div style={{ position: 'relative', width: '100%', flex: '0 0 auto' }}>
      <div style={{
        position: 'relative', maxWidth: 'var(--container-wide)', margin: '0 auto',
        padding: 'var(--space-6) var(--space-5)',
        display: 'grid', gridTemplateColumns: 'minmax(0, 1.05fr) minmax(0, 0.95fr)', gap: 'var(--space-8)', alignItems: 'start',
      }} className="tw-hero-grid">
        <div>
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8, marginBottom: 'var(--space-4)' }}>
            <span className="tw-eyebrow">Isolated labs for agent orchestrators</span>
          </div>
          <h1 style={{ fontSize: 'clamp(2.25rem, 7.5vw, var(--text-3xl))', lineHeight: 'var(--leading-tight)', marginBottom: 'var(--space-5)' }}>
            Give every orchestrator<br />its own <span className="tw-signal">runway</span>
          </h1>
          <p style={{
            fontSize: 'var(--text-md)', color: 'var(--tw-muted)', maxWidth: 460,
            lineHeight: 1.55, marginBottom: 'var(--space-6)',
          }}>
            Taxiway creates isolated labs that line up the runtime, credentials,
            workspace, tools, observability, and recordings before an orchestrator
            starts delivery work.
          </p>
          {/* DS install pill (copy on click) + amber Read the docs, per the DS hero. */}
          <div id="install" style={{ display: 'flex', gap: 'var(--space-3)', flexWrap: 'wrap', alignItems: 'center' }}>
            <InstallPill />
            <Link to="/docs" style={{
              display: 'inline-flex', alignItems: 'center', gap: 8, height: 42, padding: '0 18px',
              fontFamily: 'var(--font-display)', fontSize: 14, fontWeight: 600, borderRadius: 'var(--tw-radius)',
              background: 'var(--tw-accent)', color: 'var(--tw-accent-text)', border: '1px solid var(--tw-accent)',
              textDecoration: 'none', whiteSpace: 'nowrap',
            }}>Read the docs <Icon name="arrow" size={15} /></Link>
          </div>

          {/* supported orchestrators — sits right under the install row, per the DS */}
          <div className="tw-trust" style={{ marginTop: 28, display: 'flex', flexDirection: 'column', gap: 12 }}>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: '0.12em', textTransform: 'uppercase', color: 'var(--tw-faint)' }}>Supported orchestrators</span>
            <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-6)', flexWrap: 'wrap' }}>
              {[
                { label: 'Claude Code', to: '/docs/orchestrators/claude-code' },
                { label: 'Codex', to: '/docs/orchestrators/codex' },
                { label: 'Gas Town', to: '/docs/orchestrators/gastown' },
              ].map(o => (
                <Link key={o.label} to={o.to} className="tw-orch-link">{o.label}</Link>
              ))}
            </div>
          </div>
        </div>

        {/* quick-start terminal — comments stay next to their command; clicking a
            line copies it and shows a ✓ copied marker on the right. */}
        <div>
          <Terminal label="quick start" style={{ boxShadow: 'var(--tw-shadow-lg)' }} bodyStyle={{ overflowX: 'auto' }}>
            <QuickCmd cmd="taxiway init" note="shared runtime" copied={copiedCmd === 'taxiway init'} onCopy={() => copyCmd('taxiway init')} />
            <QuickCmd cmd="taxiway up mylab --type claude-code" note={null} copied={copiedCmd === 'taxiway up mylab --type claude-code'} onCopy={() => copyCmd('taxiway up mylab --type claude-code')} />
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 10, fontFamily: 'var(--font-mono)', fontSize: 13, lineHeight: 1.9 }}>
              {/* constrain the check to one mono cell so "start" lines up with the commands */}
              <span style={{ color: 'var(--tw-accent-2-bright)', flex: '0 0 auto', display: 'inline-block', width: '1ch', textAlign: 'center' }}>✓</span>
              <span style={{ color: 'var(--tw-muted)' }}>start</span>
            </div>
            <QuickCmd cmd="taxiway shell mylab" note="attach to the session" copied={copiedCmd === 'taxiway shell mylab'} onCopy={() => copyCmd('taxiway shell mylab')} />
          </Terminal>
          {/* status chips */}
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 16 }}>
            <Badge tone="go" dot style={{ background: 'var(--tw-surface)' }}>lima + docker</Badge>
          </div>
        </div>
      </div>
      </div>
      <div aria-hidden="true" style={{ flex: '2.5 1 0', minHeight: 'min(13vw, 185px)' }} />
    </section>
  );
}

export { Hero };

import React from 'react';
import { TWIcon } from '../icons.jsx';
import { installCmd } from '../config.js';

// How long the copied state stays before reverting.
export const COPY_FEEDBACK_MS = 1500;

/**
 * InstallPill — the design system's install affordance: a dark code pill with
 * the curl command and a COPY label on the right. Clicking anywhere copies the
 * command; the label morphs to "✓ copied" with a pop and the pill flashes a
 * teal glow. The command stays in place, so nothing on the page shifts.
 */
export function InstallPill({ style = {} }) {
  const [copied, setCopied] = React.useState(false);
  const [pulse, setPulse] = React.useState(0);
  const copy = () => {
    navigator.clipboard?.writeText(installCmd);
    setCopied(true);
    setPulse((p) => p + 1);
    setTimeout(() => setCopied(false), COPY_FEEDBACK_MS);
  };
  return (
    <button
      type="button"
      onClick={copy}
      aria-label={copied ? 'Copied install command' : 'Copy install command'}
      title="Copy install command"
      className="tw-install-pill"
      data-copied={copied ? '' : undefined}
      style={{
        display: 'inline-flex', alignItems: 'center', gap: 12, height: 42, padding: '0 14px',
        borderRadius: 'var(--tw-radius)', background: 'var(--tw-code-bg)', border: '1px solid var(--tw-code-border)',
        cursor: 'pointer', ...style,
      }}
    >
      <TWIcon name="terminal" size={15} stroke={2} color="var(--tw-accent-2-bright)" />
      <code style={{ fontFamily: 'var(--font-mono)', fontSize: 13, color: 'var(--tw-code-text)' }}>{installCmd}</code>
      {/* fixed-width label so COPY → ✓ COPIED never resizes the pill */}
      <span className="tw-install-cp" style={{
        fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: '0.12em', textTransform: 'uppercase',
        color: 'var(--tw-accent-2-bright)', minWidth: 66,
        display: 'inline-flex', justifyContent: 'flex-end', alignItems: 'center', gap: 4,
      }}>
        {copied ? (
          <span key={pulse} className="tw-copy-pop" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
            <TWIcon name="check" size={12} stroke={2.6} color="currentColor" />copied
          </span>
        ) : 'copy'}
      </span>
    </button>
  );
}

import React from 'react';

/**
 * TaxiWay CommandLine — a single `$ command  # comment` row.
 * Click copies the command; the comment swaps to a ✓ confirmation.
 * The comment shrinks + ellipsizes before the command wraps.
 */
export function CommandLine({ command, comment = null, prompt = '$', style = {}, ...rest }) {
  const [copied, setCopied] = React.useState(false);
  const copy = () => {
    const text = String(command);
    const done = () => { setCopied(true); setTimeout(() => setCopied(false), 1600); };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(done, done);
    } else { done(); }
  };
  return (
    <div
      role="button" tabIndex={0} onClick={copy}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); copy(); } }}
      title="Click to copy"
      style={{
        display: 'flex', alignItems: 'baseline', gap: 10, width: '100%',
        fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', lineHeight: 1.9,
        cursor: 'pointer', ...style,
      }}
      {...rest}
    >
      <span style={{ color: 'var(--tw-accent-2)', flex: '0 0 auto', userSelect: 'none' }}>{prompt}</span>
      <span style={{ color: 'var(--tw-code-text)', flex: '1 1 auto', minWidth: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{command}</span>
      {(comment || copied) && (
        <span style={{
          flex: '0 1 auto', minWidth: 0, textAlign: 'right',
          overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          color: copied ? 'var(--tw-accent-2)' : 'var(--tw-faint)',
          transition: 'color 130ms ease',
        }}>
          {copied ? '✓ copied' : `# ${comment}`}
        </span>
      )}
    </div>
  );
}

/**
 * TaxiWay Terminal — rounded console window: title bar (three dots + mono
 * uppercase label) over a code body. Compose CommandLine rows inside, or
 * drop in any mono content.
 */
export function Terminal({ label = 'terminal', children, dots = true, style = {}, bodyStyle = {}, ...rest }) {
  return (
    <div style={{
      borderRadius: 'calc(var(--tw-radius) + 2px)', overflow: 'hidden',
      background: 'var(--tw-code-bg)', border: '1px solid var(--tw-code-border)',
      boxShadow: 'var(--tw-shadow-md)', ...style,
    }} {...rest}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10, padding: '11px 16px',
        borderBottom: '1px solid rgba(255,255,255,0.08)',
      }}>
        {dots && (
          <span style={{ display: 'inline-flex', gap: 6 }}>
            <i style={{ width: 10, height: 10, borderRadius: '50%', background: 'var(--tw-accent)' }} />
            <i style={{ width: 10, height: 10, borderRadius: '50%', background: '#E2A437' }} />
            <i style={{ width: 10, height: 10, borderRadius: '50%', background: 'var(--tw-accent-2)' }} />
          </span>
        )}
        {label && (
          <span style={{
            fontFamily: 'var(--font-mono)', fontSize: 'var(--text-2xs)',
            letterSpacing: 'var(--tracking-label)', textTransform: 'uppercase',
            color: 'rgba(217,224,218,0.6)',
          }}>{label}</span>
        )}
      </div>
      <div style={{ padding: '16px 18px', color: 'var(--tw-code-text)', fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', ...bodyStyle }}>
        {children}
      </div>
    </div>
  );
}

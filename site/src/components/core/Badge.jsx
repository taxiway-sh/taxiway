import React from 'react';

/** TaxiWay Badge — mono status pill. Tones mirror lab status. */
export function Badge({ children, tone = 'neutral', dot = false, style = {}, ...rest }) {
  const tones = {
    neutral: { bg: 'var(--tw-surface-2)', fg: 'var(--tw-muted)', bd: 'var(--tw-border)', dot: 'var(--tw-faint)' },
    go:      { bg: 'color-mix(in srgb, var(--tw-go) 16%, transparent)', fg: 'var(--tw-go)', bd: 'color-mix(in srgb, var(--tw-go) 45%, transparent)', dot: 'var(--tw-go)' },
    hold:    { bg: 'color-mix(in srgb, var(--tw-hold) 16%, transparent)', fg: 'var(--tw-hold)', bd: 'color-mix(in srgb, var(--tw-hold) 45%, transparent)', dot: 'var(--tw-hold)' },
    alert:   { bg: 'color-mix(in srgb, var(--tw-accent) 16%, transparent)', fg: 'var(--tw-accent-ink)', bd: 'color-mix(in srgb, var(--tw-accent) 45%, transparent)', dot: 'var(--tw-accent)' },
    signal:  { bg: 'color-mix(in srgb, var(--tw-accent-2) 16%, transparent)', fg: 'var(--tw-accent-2)', bd: 'color-mix(in srgb, var(--tw-accent-2) 45%, transparent)', dot: 'var(--tw-accent-2)' },
  };
  const t = tones[tone] || tones.neutral;
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 6, padding: '3px 10px',
      borderRadius: '999px', fontFamily: 'var(--font-mono)', fontSize: 'var(--text-2xs)',
      fontWeight: 'var(--weight-medium)', letterSpacing: 'var(--tracking-wide)', textTransform: 'uppercase',
      color: t.fg, background: t.bg, border: `1px solid ${t.bd}`, whiteSpace: 'nowrap', ...style,
    }} {...rest}>
      {dot && <span style={{ width: 6, height: 6, borderRadius: '50%', background: t.dot, boxShadow: `0 0 6px ${t.dot}` }} />}
      {children}
    </span>
  );
}

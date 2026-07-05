import React from 'react';

/** TaxiWay Stat — readout tile: big number + mono label. Themed via --tw-*. */
export function Stat({ value, label, trend = null, surface = 'paper', style = {}, ...rest }) {
  const onConsole = surface === 'console';
  const trendColor = trend && trend.dir === 'down' ? 'var(--tw-accent-ink)' : 'var(--tw-accent-2)';
  return (
    <div style={{
      display: 'flex', flexDirection: 'column', gap: 4, padding: 'var(--space-5, 20px)',
      borderRadius: 'var(--tw-radius)',
      background: onConsole ? 'rgba(255,255,255,0.04)' : 'var(--tw-surface)',
      border: `1px solid ${onConsole ? 'rgba(255,255,255,0.10)' : 'var(--tw-border)'}`, ...style }} {...rest}>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
        <span style={{ fontFamily: 'var(--font-display)', fontSize: 'var(--text-2xl)',
          fontWeight: 'var(--weight-bold)', letterSpacing: 'var(--tracking-tight)',
          color: onConsole ? '#EAF1F2' : 'var(--tw-text)' }}>{value}</span>
        {trend && (
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)', color: trendColor }}>
            {trend.dir === 'down' ? '▾' : '▴'} {trend.value}
          </span>
        )}
      </div>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-2xs)',
        letterSpacing: 'var(--tracking-label)', textTransform: 'uppercase',
        color: onConsole ? 'rgba(234,241,242,0.6)' : 'var(--tw-muted)' }}>{label}</span>
    </div>
  );
}

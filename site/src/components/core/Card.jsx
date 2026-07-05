import React from 'react';

/**
 * TaxiWay Card — surface panel. paper = raised, sunken = inset,
 * console = dark block. Themed via --tw-* tokens.
 */
export function Card({ children, variant = 'paper', dome = false, padding = 'var(--space-6, 24px)', style = {}, ...rest }) {
  const variants = {
    paper:   { background: 'var(--tw-surface)', color: 'var(--tw-text)', border: '1px solid var(--tw-border)', boxShadow: 'var(--tw-shadow-md)' },
    sunken:  { background: 'var(--tw-surface-2)', color: 'var(--tw-text)', border: '1px solid var(--tw-border)', boxShadow: 'none' },
    console: { background: 'var(--tw-surface-inv)', color: '#EAF1F2', border: '1px solid rgba(0,0,0,0.4)', boxShadow: 'var(--tw-shadow-lg)' },
  };
  const v = variants[variant] || variants.paper;
  const radius = dome
    ? 'calc(var(--tw-radius) * 3) calc(var(--tw-radius) * 3) var(--tw-radius) var(--tw-radius)'
    : 'calc(var(--tw-radius) + 4px)';
  return (
    <div style={{ borderRadius: radius, padding, ...v, ...style }} {...rest}>{children}</div>
  );
}

import React from 'react';

/**
 * TaxiWay IconButton — icon-only control, themed via --tw-* tokens.
 */
export function IconButton({
  children, variant = 'ghost', size = 'md', shape = 'round',
  disabled = false, label, onClick, style = {}, ...rest
}) {
  const [hover, setHover] = React.useState(false);
  const dims = { sm: 32, md: 40, lg: 48 }[size] || 40;
  const palettes = {
    signal: { bg: hover ? 'var(--tw-accent-hover)' : 'var(--tw-accent)', fg: 'var(--tw-accent-text)', bd: 'var(--tw-accent)' },
    energy: { bg: 'var(--tw-accent-2)', fg: '#fff', bd: 'var(--tw-accent-2)' },
    console: { bg: hover ? 'color-mix(in srgb, var(--tw-surface-inv) 84%, #fff)' : 'var(--tw-surface-inv)', fg: '#EAF1F2', bd: 'var(--tw-border-strong)' },
    ghost: { bg: hover ? 'var(--tw-surface-2)' : 'transparent', fg: 'var(--tw-text)', bd: 'var(--tw-border-strong)' },
  };
  const p = palettes[variant] || palettes.ghost;
  return (
    <button
      type="button" aria-label={label} title={label}
      disabled={disabled} onClick={onClick}
      onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={{
        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
        width: dims, height: dims, padding: 0,
        color: disabled ? 'var(--tw-faint)' : p.fg,
        background: disabled ? 'var(--tw-surface-2)' : p.bg,
        border: `1px solid ${disabled ? 'var(--tw-border)' : p.bd}`,
        borderRadius: shape === 'square' ? 'var(--tw-radius)' : '999px',
        cursor: disabled ? 'not-allowed' : 'pointer',
        boxShadow: variant === 'ghost' || variant === 'console' ? 'none' : 'var(--tw-shadow-sm)',
        transition: 'background 130ms ease', ...style,
      }}
      {...rest}
    >
      {children}
    </button>
  );
}

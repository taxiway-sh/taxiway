import React from 'react';

/**
 * TaxiWay Button — control-panel action. Themed via --tw-* tokens,
 * so it adapts to Instrument (cream) or Signal (dark) automatically.
 */
export function Button({
  children, variant = 'signal', size = 'md',
  iconLeft = null, iconRight = null, disabled = false,
  type = 'button', onClick, style = {}, className = '', ...rest
}) {
  const [hover, setHover] = React.useState(false);
  const [active, setActive] = React.useState(false);

  const sizes = {
    sm: { padding: '0 14px', height: 34, fontSize: 'var(--text-sm)', gap: 6 },
    md: { padding: '0 18px', height: 42, fontSize: 'var(--text-sm)', gap: 8 },
    lg: { padding: '0 24px', height: 50, fontSize: 'var(--text-md)', gap: 10 },
  };
  const palettes = {
    signal: {
      bg: hover ? 'var(--tw-accent-hover)' : 'var(--tw-accent)',
      color: 'var(--tw-accent-text)', border: 'var(--tw-accent)',
      shadow: hover ? 'var(--tw-glow-amber), var(--tw-shadow-md)' : 'var(--tw-shadow-sm)',
    },
    energy: {
      bg: 'var(--tw-accent-2)', color: '#fff', border: 'var(--tw-accent-2)',
      shadow: hover ? 'var(--tw-glow-accent2), var(--tw-shadow-md)' : 'var(--tw-shadow-sm)',
    },
    console: {
      // Same console skin as the install pill (dark code-bg + mono) so the two
      // read as a pair: pops on the cream page, sits as a console chip on dark.
      bg: hover ? 'color-mix(in srgb, var(--tw-code-bg) 82%, #fff)' : 'var(--tw-code-bg)',
      color: '#EAF1F2', border: 'var(--tw-code-border)', shadow: 'var(--tw-shadow-sm)',
      font: 'var(--font-mono)',
    },
    ghost: {
      bg: hover ? 'var(--tw-surface-2)' : 'transparent',
      color: 'var(--tw-text)', border: 'var(--tw-border-strong)', shadow: 'none',
    },
  };
  const s = sizes[size] || sizes.md;
  const p = palettes[variant] || palettes.signal;

  return (
    <button
      type={type} disabled={disabled} onClick={onClick}
      className={`tw-btn ${className}`.trim()}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => { setHover(false); setActive(false); }}
      onMouseDown={() => setActive(true)}
      onMouseUp={() => setActive(false)}
      style={{
        display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
        gap: s.gap, height: s.height, padding: s.padding,
        fontFamily: p.font || 'var(--font-display)', fontSize: s.fontSize,
        fontWeight: 'var(--weight-semibold)', letterSpacing: '0.01em',
        color: disabled ? 'var(--tw-faint)' : p.color,
        background: disabled ? 'var(--tw-surface-2)' : p.bg,
        border: `1px solid ${disabled ? 'var(--tw-border)' : p.border}`,
        borderRadius: 'var(--tw-radius)',
        boxShadow: disabled ? 'none' : p.shadow,
        cursor: disabled ? 'not-allowed' : 'pointer',
        transform: active && !disabled ? 'translateY(1px)' : 'translateY(0)',
        transition: 'background 130ms ease, box-shadow 200ms ease, transform 120ms ease',
        whiteSpace: 'nowrap', ...style,
      }}
      {...rest}
    >
      {iconLeft}{children}{iconRight}
    </button>
  );
}

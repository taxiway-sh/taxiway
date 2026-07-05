import React from 'react';

/** TaxiWay Input — inset field with an amber focus glow. Themed via --tw-*. */
export function Input({ label = null, hint = null, iconLeft = null, invalid = false, style = {}, id, ...rest }) {
  const [focus, setFocus] = React.useState(false);
  const fieldId = id || React.useId();
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6, ...style }}>
      {label && (
        <label htmlFor={fieldId} style={{
          fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)',
          letterSpacing: 'var(--tracking-wide)', textTransform: 'uppercase',
          color: 'var(--tw-muted)', fontWeight: 'var(--weight-medium)' }}>{label}</label>
      )}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8, height: 42, padding: '0 14px',
        background: 'var(--tw-surface-2)',
        border: `1px solid ${invalid ? 'var(--tw-accent-ink)' : focus ? 'var(--tw-accent-2)' : 'var(--tw-border-strong)'}`,
        borderRadius: 'var(--tw-radius)',
        boxShadow: focus ? '0 0 0 3px color-mix(in srgb, var(--tw-accent-2) 30%, transparent)' : 'none',
        transition: 'border-color 130ms ease, box-shadow 130ms ease' }}>
        {iconLeft && <span style={{ display: 'flex', color: 'var(--tw-faint)' }}>{iconLeft}</span>}
        <input id={fieldId} onFocus={() => setFocus(true)} onBlur={() => setFocus(false)}
          style={{ flex: 1, border: 'none', outline: 'none', background: 'transparent',
            fontFamily: 'var(--font-body)', fontSize: 'var(--text-base)', color: 'var(--tw-text)' }} {...rest} />
      </div>
      {hint && <span style={{ fontSize: 'var(--text-xs)', color: invalid ? 'var(--tw-accent-ink)' : 'var(--tw-faint)' }}>{hint}</span>}
    </div>
  );
}

import React from 'react';

/** TaxiWay Switch — console toggle; glows the accent-2 (teal) when on. */
export function Switch({ checked = false, onChange, disabled = false, label = null, style = {}, ...rest }) {
  const toggle = () => { if (!disabled && onChange) onChange(!checked); };
  return (
    <label style={{ display: 'inline-flex', alignItems: 'center', gap: 10, cursor: disabled ? 'not-allowed' : 'pointer', opacity: disabled ? 0.5 : 1, ...style }}>
      <button type="button" role="switch" aria-checked={checked} onClick={toggle} disabled={disabled}
        style={{
          position: 'relative', width: 44, height: 26, flex: '0 0 auto', borderRadius: '999px', cursor: 'inherit',
          background: checked ? 'var(--tw-accent-2)' : 'var(--tw-surface-2)',
          border: `1px solid ${checked ? 'var(--tw-accent-2)' : 'var(--tw-border-strong)'}`,
          boxShadow: checked ? 'var(--tw-glow-accent2)' : 'none',
          transition: 'background 200ms ease, box-shadow 200ms ease', padding: 0 }} {...rest}>
        <span style={{
          position: 'absolute', top: 2, left: checked ? 20 : 2, width: 20, height: 20, borderRadius: '50%',
          background: 'linear-gradient(180deg,#fff,#eee)',
          boxShadow: '0 1px 3px rgba(0,0,0,0.3), inset 0 1px 0 rgba(255,255,255,0.8)',
          transition: 'left 200ms cubic-bezier(0.2,0.8,0.2,1)' }} />
      </button>
      {label && <span style={{ fontSize: 'var(--text-sm)', color: 'var(--tw-text)' }}>{label}</span>}
    </label>
  );
}

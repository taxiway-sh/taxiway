import React from 'react';

/** TaxiWay Tag — mono metadata chip (labs, drivers, regions). Themed via --tw-*. */
export function Tag({ children, onRemove = null, icon = null, style = {}, ...rest }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 6, padding: '4px 10px',
      borderRadius: 'var(--tw-radius)', fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)',
      color: 'var(--tw-text-2)', background: 'var(--tw-surface)', border: '1px solid var(--tw-border)',
      whiteSpace: 'nowrap', ...style }} {...rest}>
      {icon && <span style={{ display: 'flex', color: 'var(--tw-accent-2)' }}>{icon}</span>}
      {children}
      {onRemove && (
        <button type="button" onClick={onRemove} aria-label="Remove" style={{
          display: 'inline-flex', alignItems: 'center', justifyContent: 'center',
          width: 14, height: 14, marginLeft: 2, padding: 0, border: 'none', borderRadius: '50%',
          cursor: 'pointer', background: 'transparent', color: 'var(--tw-faint)', fontSize: 12, lineHeight: 1 }}>×</button>
      )}
    </span>
  );
}

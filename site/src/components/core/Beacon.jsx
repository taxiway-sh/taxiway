import React from 'react';

// THE brand mark: the rich beacon lamp (taxiway-beacon-v2.png), served at the
// site root as /icon.png, wrapped in a CSS pulse halo + amber drop-shadow glow.
// One asset, reads on both cream and dark grounds. Override with `src`.
// (Build note: the design's inline-SVG Beacon is the schematic variant; the
// site uses the rich rendered lamp per the brand-logo guideline.)
const BEACON_SRC = '/icon.png?v=20260704';

/**
 * TaxiWay Beacon — the signature mark, animated. Renders the rich beacon logo
 * wrapped in a pulsing amber halo. Use as the brand glyph, a "live" indicator,
 * or a status light.
 */
export function Beacon({ size = 48, live = true, label = null, src = BEACON_SRC, style = {}, ...rest }) {
  const bloom = Math.max(4, Math.round(size * 0.16));
  return (
    <span role="img" aria-label={label || 'TaxiWay beacon'}
      style={{ display: 'inline-flex', alignItems: 'center', gap: 10, ...style }} {...rest}>
      <span style={{ position: 'relative', width: size, height: size, flex: '0 0 auto', display: 'inline-block' }}>
        {/* pulsing amber halo behind the lamp (not baked into the asset) */}
        <span style={{
          position: 'absolute', inset: '-30%', borderRadius: '50%', pointerEvents: 'none',
          background: 'radial-gradient(circle at 50% 42%, rgba(245,160,42,0.55) 0%, transparent 62%)',
          animation: live ? 'tw-beacon-pulse 1.6s ease-in-out infinite' : 'none',
        }} />
        <img src={src} alt="" width={size} height={size} style={{
          position: 'relative', display: 'block', width: size, height: size,
          filter: live ? `drop-shadow(0 0 ${bloom}px rgba(245,160,42,0.55))` : 'none',
        }} />
      </span>
      {label && (
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-xs)',
          letterSpacing: 'var(--tracking-label)', textTransform: 'uppercase', color: 'var(--tw-muted)' }}>{label}</span>
      )}
    </span>
  );
}

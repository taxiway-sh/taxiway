import React from 'react';

/**
 * TaxiWay ThemeToggle — the round beacon-lens switch from the landing kit.
 * A top-down amber lens that sits dimmed in the light theme and lights up
 * (amber glow) in the dark (Signal) theme. Toggles data-theme on <html>,
 * persists to localStorage, and defaults to prefers-color-scheme.
 * The lit/dimmed appearance is CSS-driven off [data-theme] (see page.css),
 * so it always matches the active theme with no hydration flicker.
 */
export function ThemeToggle({ size = 40, style = {}, ...rest }) {
  const read = () => {
    if (typeof document === 'undefined') return 'light';
    return document.documentElement.dataset.theme
      || (typeof localStorage !== 'undefined' && localStorage.getItem('tw-theme'))
      || (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'signal' : 'light');
  };
  const [theme, setTheme] = React.useState(read);

  React.useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try { localStorage.setItem('tw-theme', theme); } catch (e) {}
  }, [theme]);

  const dark = theme === 'signal';
  const toggle = () => setTheme(dark ? 'light' : 'signal');

  return (
    <button
      type="button" role="switch" aria-checked={dark}
      aria-label={dark ? 'Switch to day shift (light)' : 'Switch to night ops (dark)'}
      title={dark ? 'Night ops' : 'Day shift'}
      onClick={toggle}
      className="tw-tt"
      style={{ width: size, height: size, ...style }}
      {...rest}
    >
      <span className="tw-tt-lens" aria-hidden="true" />
    </button>
  );
}

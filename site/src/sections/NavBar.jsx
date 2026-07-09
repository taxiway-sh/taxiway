/* Taxiway landing — top navigation bar */
import React from 'react';
import { Button, IconButton, Beacon, ThemeToggle } from '../components/core';
import { TWIcon } from '../icons.jsx';
import { repoUrl } from '../config.js';
import { Link } from 'react-router-dom';

function NavBar() {
  const Icon = TWIcon;
  const [open, setOpen] = React.useState(false);
  const headerRef = React.useRef(null);
  // Section anchors (they scroll within the landing) live on the left; Docs is
  // a route — it leaves the landing — so it sits on the right, with GitHub.
  const sectionLinks = [
    { label: 'Lifecycle', to: '/#lifecycle' },
    { label: 'Features', to: '/#features' },
    { label: 'Control tower', to: '/#control-tower' },
  ];
  const docsLink = { label: 'Docs', to: '/docs', icon: 'book' };

  // Close the mobile menu when the viewport grows back to desktop.
  React.useEffect(() => {
    if (typeof window.matchMedia !== 'function') return undefined;
    const mq = window.matchMedia('(min-width: 921px)');
    const onChange = (e) => { if (e.matches) setOpen(false); };
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  // Close on Escape for keyboard users.
  React.useEffect(() => {
    if (!open) return undefined;
    const onKey = (e) => { if (e.key === 'Escape') setOpen(false); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open]);

  // iOS Safari does not repaint a sticky element with backdrop-filter when the
  // theme changes its translucent background — the blurred layer stays cached
  // until a scroll forces a re-composite (the header keeps the old theme's
  // colour). On every data-theme flip, briefly drop and restore the filter to
  // force the compositor to re-render the header with the new colour.
  React.useEffect(() => {
    const root = document.documentElement;
    const repaint = () => {
      const el = headerRef.current;
      if (!el) return;
      el.style.backdropFilter = 'none';
      el.style.webkitBackdropFilter = 'none';
      requestAnimationFrame(() => requestAnimationFrame(() => {
        if (!headerRef.current) return;
        headerRef.current.style.backdropFilter = 'blur(10px)';
        headerRef.current.style.webkitBackdropFilter = 'blur(10px)';
      }));
    };
    const obs = new MutationObserver(repaint);
    obs.observe(root, { attributes: true, attributeFilter: ['data-theme'] });
    return () => obs.disconnect();
  }, []);

  return (
    <header ref={headerRef} style={{
      position: 'sticky', top: 0, zIndex: 50,
      borderBottom: '1px solid var(--tw-border)',
      background: 'var(--tw-nav-bg)',
      backdropFilter: 'blur(10px)',
      WebkitBackdropFilter: 'blur(10px)',
    }}>
      <nav style={{
        maxWidth: 'var(--container-wide)', margin: '0 auto',
        height: 'var(--nav-height)', padding: '0 var(--space-5)',
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-6)' }}>
          <Link to="/" onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
            style={{ display: 'flex', alignItems: 'center', gap: 10, textDecoration: 'none' }}>
            <Beacon size={36} />
            <span style={{
              fontFamily: 'var(--font-logo)', fontWeight: 700, fontSize: 21,
              letterSpacing: '-0.01em', color: 'var(--tw-text)',
            }}>taxiway</span>
            {/* subtle outlined pill, per the DS */}
            <span style={{
              fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: '0.08em', textTransform: 'uppercase',
              padding: '2px 8px', borderRadius: 999, border: '1px solid var(--tw-border)', color: 'var(--tw-accent-2)',
              marginLeft: 2,
            }}>Beta</span>
          </Link>
          <ul style={{
            display: 'flex', gap: 'var(--space-5)', listStyle: 'none', margin: 0, padding: 0,
          }} className="tw-navlinks">
            {sectionLinks.map(l => (
              <li key={l.label}>
                <Link to={l.to} style={{
                  fontFamily: 'var(--font-body)', fontSize: 'var(--text-sm)',
                  fontWeight: 500, color: 'var(--tw-muted)', textDecoration: 'none',
                }}>{l.label}</Link>
              </li>
            ))}
          </ul>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
          {/* Light/dark switch — always visible (desktop + mobile). */}
          <ThemeToggle size={38} />
          <span className="tw-navlinks" style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-5)' }}>
            <Link to={docsLink.to} style={{
              display: 'inline-flex', alignItems: 'center', gap: 6,
              fontFamily: 'var(--font-body)', fontSize: 'var(--text-sm)',
              fontWeight: 500, color: 'var(--tw-muted)', textDecoration: 'none',
            }}><Icon name={docsLink.icon} size={15} />{docsLink.label}</Link>
            <a href={repoUrl} target="_blank" rel="noreferrer" style={{ textDecoration: 'none' }}>
              <Button variant="console" size="sm" iconLeft={<Icon name="git" size={16} />}>GitHub</Button>
            </a>
          </span>
          {/* Burger — only shown on small screens (see page.css media query). */}
          <span className="tw-burger">
            <IconButton
              label={open ? 'Close menu' : 'Open menu'}
              variant="ghost"
              shape="square"
              aria-expanded={open}
              aria-controls="tw-mobile-menu"
              onClick={() => setOpen(o => !o)}
            >
              <Icon name={open ? 'close' : 'menu'} size={20} />
            </IconButton>
          </span>
        </div>
      </nav>

      {/* Mobile dropdown panel */}
      {open && (
        <div id="tw-mobile-menu" className="tw-mobile-menu" style={{
          borderTop: '1px solid var(--tw-border)',
          background: 'var(--tw-surface)',
          boxShadow: 'var(--tw-shadow-md)',
        }}>
          <ul style={{
            listStyle: 'none', margin: 0, padding: 'var(--space-3) var(--space-5)',
            display: 'flex', flexDirection: 'column',
          }}>
            {[...sectionLinks, docsLink].map(l => (
              <li key={l.label}>
                <Link to={l.to} onClick={() => setOpen(false)} style={{
                  display: 'flex', alignItems: 'center', gap: 10, padding: '12px 4px',
                  borderBottom: '1px solid var(--tw-border)',
                  fontFamily: 'var(--font-body)', fontSize: 'var(--text-md)',
                  fontWeight: 500, color: 'var(--tw-text-2)', textDecoration: 'none',
                }}>{l.icon && <Icon name={l.icon} size={17} />}{l.label}</Link>
              </li>
            ))}
          </ul>
          <div style={{ padding: '0 var(--space-5) var(--space-5)' }}>
            <a href={repoUrl} target="_blank" rel="noreferrer"
              onClick={() => setOpen(false)} style={{ textDecoration: 'none' }}>
              <Button variant="console" size="md" iconLeft={<Icon name="git" size={16} />} style={{ width: '100%' }}>GitHub</Button>
            </a>
          </div>
        </div>
      )}
    </header>
  );
}
export { NavBar };

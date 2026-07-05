import React from 'react';
import { Link } from 'react-router-dom';
import { NavBar } from '../sections/NavBar.jsx';
import { Footer } from '../sections/CtaSection.jsx';
import { Button, Beacon } from '../components/core';
import { TWIcon } from '../icons.jsx';
import '../styles/page.css';

export function NotFound() {
  return (
    <div className="tw-page-bg" style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      <NavBar />
      <section style={{
        flex: 1, width: '100%', maxWidth: 640, margin: '0 auto', padding: 'var(--space-6) var(--space-5)',
        textAlign: 'center',
        display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
        gap: 'var(--space-4)',
      }}>
        <Beacon size={64} />
        <span style={{
          fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', letterSpacing: '0.16em',
          textTransform: 'uppercase', color: 'var(--tw-faint)',
        }}>Error 404</span>
        <h1 style={{ fontSize: 'var(--text-2xl)', margin: 0 }}>This page took a wrong turn.</h1>
        <p style={{ fontSize: 'var(--text-md)', color: 'var(--tw-muted)', maxWidth: 460, margin: 0, lineHeight: 1.55 }}>
          The page you are looking for does not exist or has moved. Head back home
          or browse the documentation.
        </p>
        <div style={{ display: 'flex', gap: 'var(--space-3)', flexWrap: 'wrap', justifyContent: 'center' }}>
          {/* Quiet secondary; the amber "Read the docs" (landing twin) carries the emphasis. */}
          <Link to="/" style={{ textDecoration: 'none' }}>
            <Button variant="ghost" size="md">Back home</Button>
          </Link>
          <Link to="/docs" style={{ textDecoration: 'none' }}>
            <Button variant="signal" size="md" iconRight={<TWIcon name="arrow" size={15} />}>Read the docs</Button>
          </Link>
        </div>
      </section>
      <Footer />
    </div>
  );
}

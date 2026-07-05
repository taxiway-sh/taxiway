import React from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { DocsNav } from '../DocsNav.jsx';
import { docs } from '../loader.js';

describe('DocsNav', () => {
  it('renders a link for every doc page', () => {
    render(<MemoryRouter><DocsNav current="/docs/reference/concepts" /></MemoryRouter>);
    for (const d of docs) {
      expect(screen.getByRole('link', { name: d.title })).toBeTruthy();
    }
  });

  it('marks the current page active', () => {
    render(<MemoryRouter><DocsNav current="/docs/reference/concepts" /></MemoryRouter>);
    const active = document.querySelector('a.is-active');
    expect(active).toBeTruthy();
    expect(active.getAttribute('aria-current')).toBe('page');
  });
});

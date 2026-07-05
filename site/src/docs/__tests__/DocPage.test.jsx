import React from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { DocPage } from '../DocPage.jsx';

const doc = {
  rel: 'reference/concepts',
  route: '/docs/concepts',
  group: 'Reference',
  title: 'Concepts',
  raw: '# Concepts\n\nA **lab** is an isolated environment.\n\n## Set up\n\nSteps here.\n\nRun it:\n\n```sh\ntaxiway up mylab\n```\n\nSee [Commands](commands.md) and [GitHub](https://github.com/x/y).\n',
};

describe('DocPage', () => {
  it('renders the markdown H1 as a heading', () => {
    const { container } = render(<MemoryRouter><DocPage doc={doc} /></MemoryRouter>);
    const h1 = container.querySelector('h1');
    expect(h1).toBeTruthy();
    expect(h1.textContent).toContain('Concepts');
  });

  it('renders markdown body content', () => {
    const { container } = render(<MemoryRouter><DocPage doc={doc} /></MemoryRouter>);
    expect(container.textContent).toContain('isolated environment');
  });

  it('adds a copy button to fenced code blocks', () => {
    render(<MemoryRouter><DocPage doc={doc} /></MemoryRouter>);
    expect(screen.getByRole('button', { name: /copy code/i })).toBeTruthy();
  });

  it('gives headings slug ids so in-page anchors have targets', () => {
    const { container } = render(<MemoryRouter><DocPage doc={doc} /></MemoryRouter>);
    expect(container.querySelector('h1#concepts')).toBeTruthy();
  });

  it('renders an anchor-link icon on section headings', () => {
    const { container } = render(<MemoryRouter><DocPage doc={doc} /></MemoryRouter>);
    const anchor = container.querySelector('h2#set-up .tw-heading-anchor');
    expect(anchor).toBeTruthy();
    expect(anchor.getAttribute('href')).toBe('#set-up');
  });

  it('rewrites a repo-relative .md link in the body to an internal route', () => {
    const { container } = render(<MemoryRouter><DocPage doc={doc} /></MemoryRouter>);
    const article = within(container.querySelector('.tw-prose'));
    const link = article.getByRole('link', { name: 'Commands' });
    expect(link.getAttribute('href')).toBe('/docs/reference/commands');
    expect(link.getAttribute('target')).toBeNull();
  });

  it('opens external body links in a new tab', () => {
    const { container } = render(<MemoryRouter><DocPage doc={doc} /></MemoryRouter>);
    const article = within(container.querySelector('.tw-prose'));
    const link = article.getByRole('link', { name: 'GitHub' });
    expect(link.getAttribute('href')).toBe('https://github.com/x/y');
    expect(link.getAttribute('target')).toBe('_blank');
  });
});

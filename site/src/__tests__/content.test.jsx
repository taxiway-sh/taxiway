import React from 'react';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import App from '../App.jsx';

const renderApp = () => render(<MemoryRouter><App /></MemoryRouter>);

const FORBIDDEN = [
  '100k jobs', 'credit card', 'runway fees', 'Northwind', 'Helio',
  'Switchyard', 'Polestar', 'Cargot', 'SOC 2', 'dead-letter',
  'checkout-svc', 'orchestration layer', 'queues, pipelines, and workloads',
  'Runway', 'Sort', 'Hold', 'Jobs arrive', 'Smart routing',
  'Queues & retries', 'Execute & ship', 'at-least-once', 'backpressure-aware',
  'priority lanes', 'zero-config retries',
  'Holds & retries', 'Live observability', 'Secrets & scopes', 'Any workload', 'Safe deploys',
  'In flight', 'On-time', 'p50 latency', 'Held lanes', 'Auto-route',
  'Get started free', 'control tower for your background work',
  'Routing since the runway opened', 'TaxiWay, Inc.',
];

test('page renders without invented marketing copy', () => {
  const { container } = renderApp();
  const text = container.textContent;
  for (const phrase of FORBIDDEN) {
    expect(text).not.toContain(phrase);
  }
});

test('navbar renders GitHub CTA', () => {
  renderApp();
  expect(screen.getByRole('button', { name: 'GitHub' })).toBeTruthy();
});

test('page states the real product positioning', () => {
  const { container } = renderApp();
  const text = container.textContent;
  expect(text).toMatch(/isolated labs/i);
  expect(text).toMatch(/its own runway/i);
  expect(text).toContain('curl -fsSL https://taxiway.run');
  expect(text).toContain('taxiway init');
});

test('pipeline section shows real taxiway phases', () => {
  const { container } = renderApp();
  const text = container.textContent;
  expect(text).toContain('Verify');
  expect(text).not.toContain('Runway');
});

test('feature grid shows real taxiway capabilities', () => {
  const { container } = renderApp();
  const text = container.textContent;
  expect(text).toContain('Multi Orchestrators');
  expect(text).toContain('Langfuse');
  expect(text).toContain('Isolated Sandboxes');
  expect(text).toContain('Disposable Labs');
  expect(text).toContain('Gateway');
  expect(text).toContain('Observability');
  expect(text).toContain('Recordings');
});

test('console preview shows real taxiway status content', () => {
  const { container } = renderApp();
  const text = container.textContent;
  expect(screen.getByText('taxiway-mylab')).toBeTruthy();
  expect(text).toContain('taxiway-mylab');
  expect(text).toContain('Running');
  expect(text).toContain('Stopped');
  expect(text).toContain('Control tower');
  expect(text).toContain('Driver: Auto');
});

test('cta and footer show real taxiway content and curated links', () => {
  const { container } = renderApp();
  const text = container.textContent;
  expect(text).toContain('Spin up your first lab');
  expect(text).toContain('Gate to runway.');
  expect(text).toContain('© 2026 Manufacture');
  // Footer: Orchestrators
  expect(text).toContain('Claude Code');
  expect(text).toContain('Codex');
  expect(text).toContain('Gas Town');
  // Footer: Documentation
  expect(text).toContain('Overview');
  expect(text).toContain('Concepts');
  expect(text).toContain('CLI Usage');
  // Footer: Project
  expect(text).toContain('GitHub');
  expect(text).toContain('License');
});

import React from 'react';
import { Navigate } from 'react-router-dom';
import App from '../App.jsx';
import { DocPage } from './DocPage.jsx';
import { NotFound } from './NotFound.jsx';
import { docs, categories } from './loader.js';

export const routes = [
  { path: '/', element: <App />, entry: 'src/App.jsx' },
  ...docs.map((doc) => ({
    path: doc.route,
    element: <DocPage doc={doc} />,
    entry: 'src/docs/DocPage.jsx',
  })),
  // Bare category paths (/docs/orchestrators, ...) have no page of their own —
  // redirect to the matching section of the overview page.
  ...categories.map((cat) => ({
    path: `/docs/${cat}`,
    element: <Navigate to={`/docs#${cat}`} replace />,
  })),
  // 404: a concrete /404 route (emitted as dist/404.html for GitHub Pages) plus
  // a client-side catch-all for unknown paths.
  { path: '/404', element: <NotFound />, entry: 'src/docs/NotFound.jsx' },
  { path: '*', element: <NotFound />, entry: 'src/docs/NotFound.jsx' },
];

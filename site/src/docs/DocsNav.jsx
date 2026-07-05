import React from 'react';
import { Link } from 'react-router-dom';
import { navGroups } from './loader.js';

export function DocsNav({ current, currentGroup, open = false, onClose }) {
  return (
    <aside id="tw-docs-nav" className={`tw-docs-nav${open ? ' is-open' : ''}`}>
      {navGroups.map((group) =>
        group.name === '' ? (
          // Index: a clickable section-level entry, peer of the section headings.
          group.pages.map((p) => (
            <Link
              key={p.route}
              to={p.route}
              className={`tw-docs-nav-section-link${p.route === current ? ' is-active' : ''}`}
              aria-current={p.route === current ? 'page' : undefined}
              onClick={onClose}
            >
              {p.title}
            </Link>
          ))
        ) : (
          <div key={group.name} className="tw-docs-nav-group">
            <h4 className={`tw-docs-nav-h${group.name === currentGroup ? ' is-active' : ''}`}>{group.name}</h4>
            <ul>
              {group.pages.map((p) => (
                <li key={p.route}>
                  <Link
                    to={p.route}
                    className={p.route === current ? 'is-active' : undefined}
                    aria-current={p.route === current ? 'page' : undefined}
                    onClick={onClose}
                  >
                    {p.title}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        )
      )}
    </aside>
  );
}

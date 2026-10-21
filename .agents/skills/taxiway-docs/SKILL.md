---
name: taxiway-docs
description: Use when writing, moving, renaming or reviewing Taxiway documentation under docs/ or the landing site under site/, including page titles, links, navigation, Mermaid diagrams and landing copy.
---

# Taxiway documentation

Owns how Taxiway's documentation is written so it renders on GitHub and on the
site: the pages under `docs/` and the landing page under `site/`. Read
[`AGENTS.md`](../../../AGENTS.md) first. What a page says belongs to the owner
of its topic (see the routing table); this skill owns its form, links and
place in the navigation. The checks a change needs are chosen with
[`taxiway-testing`](../taxiway-testing/SKILL.md), and anything published
follows [public writing](../taxiway-security/SKILL.md#public-writing).

## Where pages live

- `docs/` holds the user documentation and `docs/contributing/` the
  contributor documentation. The site renders every `docs/**/*.md` page from
  the repository at build time
  ([`loader.js`](../../../site/src/docs/loader.js)); there is no second copy.
- A page's route mirrors its path: `docs/<folder>/<page>.md` is
  `/docs/<folder>/<page>`, and `docs/README.md` is `/docs`, the overview.
- `docs/contributing/README.md` is the contributing index for GitHub only. The
  site leaves it out and shows the overview's Contributing section instead:
  links to it resolve to `/docs#contributing`, and `/docs/contributing/README`
  redirects there ([`routes.jsx`](../../../site/src/docs/routes.jsx)).
- The repository `README.md`, `CONTRIBUTING.md`, `AGENTS.md` and the skills
  render on GitHub only.

## Dual rendering

Every page under `docs/` renders correctly both on GitHub and on the site. The
site renders with react-markdown, `remark-gfm` and `rehype-slug`
([`DocPage.jsx`](../../../site/src/docs/DocPage.jsx)), so only syntax that both
support works:

- **Markdown and GFM only:** headings, lists, tables, code fences, links,
  emphasis. Code fences carry a language for highlighting; a `mermaid` fence
  becomes a [diagram](#mermaid-diagrams).
- **No raw HTML.** The site prints it as literal text: `<a id>`, `<details>`,
  `<br>` and `<img>` show their tags and create no anchor. Use headings as
  anchors.
- **No GitHub-only syntax:** alerts (`> [!NOTE]`) show their marker as text on
  the site, and YAML front matter renders as a rule and a heading.
- **No images by relative path.** The site serves `site/public/`, not
  `docs/assets/`; `docs/assets/` is for the repository `README.md`.
- **One H1, first.** The first `# ` line is the page title in the sidebar, the
  pager and the overview ([Titles](#titles)).

## Links

[`resolveDocLink`](../../../site/src/docs/loader.js) rewrites each link of a
page body for the site:

- **Between docs pages:** a relative `.md` link, such as `page.md`,
  `../drivers/docker.md` or `../README.md#drivers`. GitHub follows it as a
  file, and the site turns it into the page's route with the same anchor. Do
  not write site routes (`/docs/...`) in a page: GitHub cannot follow them.
- **To a file outside `docs/`:** an absolute GitHub URL,
  `https://github.com/taxiway-sh/taxiway/blob/main/<path>` (`tree/main/` for a
  folder). A relative link that leaves `docs/`, or that is not a `.md` page,
  reaches nothing on the site; `make lint` rejects the first.
- **Anchors** are heading slugs, which GitHub and the site compute the same
  way (github-slugger): lowercase, punctuation dropped, spaces become `-`, so
  ``## Run `taxiway up` `` is `#run-taxiway-up`. Renaming a heading breaks its
  anchors: search the repository, skills and blob URLs included, for
  `<page>.md#<old-slug>` and update them.
- **Contributing index:** link to `contributing/README.md` without an anchor;
  its sections do not exist on the site, which keeps only `#contributing`.

The [link test](../../../site/src/docs/__tests__/links.test.jsx) walks every
link and image of every docs page and fails on a link that reaches no page,
drops or misses its anchor, or points to a file the site does not serve, and on
an image by relative path.

## Navigation

The sidebar, the overview page and the contributing index list the same pages,
in the same order, under the same titles;
[`navigation.test.js`](../../../site/src/docs/__tests__/navigation.test.js)
enforces it.

- **A new page** goes into `PAGE_ORDER` in
  [`loader.js`](../../../site/src/docs/loader.js) at its place in the reader's
  path, into its section of [`docs/README.md`](../../../docs/README.md), and,
  for a contributing page, into the table of
  [`docs/contributing/README.md`](../../../docs/contributing/README.md) with
  its purpose. The overview and the index link with a bare `<folder>/<page>.md`
  (in the index, `<page>.md`): no `./` prefix and no anchor.
- **Link text equals the page's H1** in the overview and the index; links
  elsewhere that name the page by its title use the H1 too.
- **A renamed title** updates those link texts in the same change.
- **A new folder** is a new sidebar section: add it to `groupFor` and
  `GROUP_ORDER` in `loader.js` (the loader throws on an unknown folder), to
  `SECTIONS` in `navigation.test.js`, and as a `## <Section>` heading of the
  overview whose slug is the folder name, because `/docs/<folder>` redirects to
  `/docs#<folder>`.

## Titles

- An H1 names the page in a few words and fits on one sidebar line: at most
  28 characters, for a 232px sidebar column
  ([`docs.css`](../../../site/src/docs/docs.css)). The navigation test checks
  the limit. The overview's H1 shows as "Overview" in the sidebar.
- File names and routes stay stable when a title changes: routes are public
  URLs, linked from the landing page, other sites and bookmarks. A page whose
  H1 changed keeps its file
  ([#246](https://github.com/taxiway-sh/taxiway/pull/246) renamed
  `agentic-system.md` to "Project agents" and kept the file).
- A file that must move updates every inbound link in the same change: the
  docs, the skills, blob URLs, `PAGE_ORDER` and the landing links under
  `site/src/` with their tests in
  [`content.test.jsx`](../../../site/src/__tests__/content.test.jsx).

## Mermaid diagrams

GitHub renders a `mermaid` fence with its own Mermaid version. The site renders
it in the browser with Mermaid 11, `securityLevel: 'strict'` and a theme taken
from the site's colour tokens, and shows the source as code when it fails to
parse ([`Mermaid.jsx`](../../../site/src/docs/Mermaid.jsx)).

- Labels are plain text: `id[Label]`, or `id["Label: with punctuation"]` in
  quotes. Write `<` and `>` as `&lt;` and `&gt;`, as
  [Architecture](../../../docs/reference/architecture.md) does.
- No Markdown in labels (backtick strings, `**bold**`) and no HTML (`<br>`,
  `<b>`): GitHub and the strict site renderer treat them differently.
- No `%%{init}%%` directives, `style` or `classDef` colours, or `click` links:
  the site themes diagrams for light and dark, and strict mode drops links.
- Prefer `flowchart`, the type every existing diagram uses.
- Neither the tests nor `npm run build` render diagrams. Check a new or changed
  diagram on GitHub (the PR's rich diff) and on the site (`npm run dev`).

## Landing copy

The landing page (`site/src/App.jsx` and `site/src/sections/`) is public.

- It describes Taxiway as it is: real commands, lifecycle phases, orchestrators
  and capabilities, each linking to its docs page. No invented features,
  metrics, customers, testimonials or pricing;
  [`content.test.jsx`](../../../site/src/__tests__/content.test.jsx) asserts the
  real positioning and forbids the template copy it replaced. Extend it when
  the copy changes.
- No private, client or personal data: examples use generic names such as
  `mylab`. No project or organization other than Taxiway, its maintainer and
  its documented dependencies and orchestrators.
- English, like every repository artifact.
- Links go to doc routes (`/docs/...`, `/docs#<section>`) or to the URLs built
  in [`config.js`](../../../site/src/config.js).

## Checks

- **Any change under `docs/` or `site/`:** the site checks,
  `npm ci`, `npm test` and `npm run build` in `site/`
  ([Testing](../../../docs/contributing/testing.md#local-checks)). Run them
  before the PR and report them in its Validation section; the `site-checks`
  job of `ci.yml` runs them again on the PR
  ([CI structure](../../../docs/contributing/testing.md#ci-structure)).
- **Any Markdown change:** `make lint`, whose `check-agent-docs` checks
  relative links, anchors and the blob URL rule in every page and skill.
- **Not covered by tests:** diagram rendering and visual wrapping. Look at the
  page with `npm run dev` when a change affects them.

## Known gaps

- `check-agent-docs` computes anchors with its own `slug()`, an approximation
  of github-slugger: it ignores setext headings and a duplicate suffix that
  collides with another heading, and it accepts `<a id>` anchors that the site
  does not create. For pages under `docs/` the site link test is authoritative; skills
  and `AGENTS.md` rely on `check-agent-docs` alone, so keep their headings
  plain.

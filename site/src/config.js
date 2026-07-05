// Single source of truth for GitHub / install URLs.
export const REPO = 'taxiway-sh/taxiway';
export const repoUrl = `https://github.com/${REPO}`;
// taxiway.run 302-redirects to the latest GitHub release's install.sh asset
// (curl -fsSL follows redirects). GitHub stays the single source; the `| sh`
// already signals it's a shell script, so the URL stays a bare domain.
export const installUrl = 'https://taxiway.run';
export const installCmd = `curl -fsSL ${installUrl} | sh`;
export const docUrl = (path) => `${repoUrl}/blob/main/${path}`;

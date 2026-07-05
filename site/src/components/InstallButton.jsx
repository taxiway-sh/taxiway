import React from 'react';
import { Button } from './core';
import { TWIcon } from '../icons.jsx';
import { installCmd } from '../config.js';

// How long the "Copied!" feedback stays before reverting.
export const COPY_FEEDBACK_MS = 1400;

// Fixed width that fits the install command so switching to "Copied!" doesn't
// shrink the button and shift its neighbours.
const MIN_WIDTH = 386;

// Primary CTA that shows the install one-liner and copies it on click.
export function InstallButton() {
  const [copied, setCopied] = React.useState(false);
  const copy = () => {
    navigator.clipboard?.writeText(installCmd);
    setCopied(true);
    setTimeout(() => setCopied(false), COPY_FEEDBACK_MS);
  };
  return (
    <Button
      variant="signal"
      size="lg"
      className="tw-install-btn"
      iconLeft={<TWIcon name={copied ? 'check' : 'terminal'} size={18} />}
      onClick={copy}
      title="Copy install command"
      style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)', minWidth: MIN_WIDTH }}
    >
      {copied ? 'Copied!' : installCmd}
    </Button>
  );
}

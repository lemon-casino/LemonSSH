import assert from 'node:assert/strict';
import test from 'node:test';
import { renderToStaticMarkup } from 'react-dom/server';

import { AppWordmark } from './AppWordmark';

test('AppWordmark renders the LemonSSH product name', () => {
  const markup = renderToStaticMarkup(<AppWordmark className="h-5" />);
  assert.match(markup, />LemonSSH</);
  assert.match(markup, /aria-label="LemonSSH"/);
  assert.match(markup, /class="h-5"/);
});

test('AppWordmark exposes an accessible product name when requested', () => {
  const markup = renderToStaticMarkup(
    <AppWordmark accessibleLabel="LemonSSH" className="h-8" />,
  );
  assert.match(markup, /aria-label="LemonSSH"/);
  assert.match(markup, /role="img"/);
});

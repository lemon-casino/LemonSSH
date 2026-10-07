import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { createDomRenderer, installDomEnvironment, runWithAct } from '../test-support/renderReactDom';
import { installTreeEnvironmentMocks } from './testEnvironmentMocks';

test('workbench width clamps and expansion survives remount', async () => {
  const env = installDomEnvironment();
  const restore = installTreeEnvironmentMocks();
  const { useWorkbenchTreeWidth, useWorkbenchTreeExpanded } = await import('../../application/state/workbenchSessionTreeStore');
  let state: ReturnType<typeof useWorkbenchTreeWidth>;
  let tree: ReturnType<typeof useWorkbenchTreeExpanded>;
  function Probe() {
    state = useWorkbenchTreeWidth();
    tree = useWorkbenchTreeExpanded();
    return <output>{state.width}:{Array.from(tree.expandedPaths).join(',')}</output>;
  }
  let renderer = await createDomRenderer(env.document);
  try {
    await renderer.render(<Probe />);
    await runWithAct(() => { state.resize(999); tree.ensurePathExpanded('Prod/Web'); });
    assert.equal(renderer.container.textContent, '480:Prod/Web');
    await renderer.unmount();
    renderer = await createDomRenderer(env.document);
    await renderer.render(<Probe />);
    assert.equal(renderer.container.textContent, '480:Prod/Web');
    await runWithAct(() => state.resize(-100));
    assert.equal(renderer.container.textContent, '180:Prod/Web');
  } finally {
    await renderer.unmount();
    restore();
    env.cleanup();
  }
});

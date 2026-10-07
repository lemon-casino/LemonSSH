/**
 * Integration test for the workbench group-delete flow.
 *
 * Exercises `useVaultGroupDeletion` against a commit implementation that
 * mirrors `useVaultState.commitVaultGroupMutation`, pinning the contract the
 * transport layer must satisfy: deleting a group with `deleteHosts=true` must
 * apply in a single transaction and finish (no supersede loop).
 *
 * The supersede-loop regression itself lives in
 * `workbenchGroupDeleteConcurrent.test.tsx` (which injects a concurrent
 * re-render), and the snapshot comparison in `vaultSnapshotGuard.test.ts`.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';

import type { GroupConfig, Host, ManagedSource } from '../../domain/models.ts';
import { buildVaultGroupDeletion } from '../../domain/vaultGroupDeletion.ts';
import { useVaultGroupDeletion } from './useVaultGroupDeletion.ts';

type VaultState = {
  groups: string[];
  configs: GroupConfig[];
  hosts: Host[];
  managedSources: ManagedSource[];
  snippets: unknown[];
};

const makeHost = (id: string, group: string): Host => ({
  id,
  label: id,
  hostname: '10.0.0.1',
  username: 'root',
  port: 22,
  protocol: 'ssh',
  group,
}) as Host;

test('delete-group-with-hosts applies in one transaction when the snapshot matches', async () => {
  const actEnvironment = globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT?: boolean;
  };
  const previousActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT;
  actEnvironment.IS_REACT_ACT_ENVIRONMENT = true;

  // Persisted vault: two groups, hosts inside the one being deleted.
  let persisted: VaultState = {
    groups: ['Prod', 'Prod/Web', 'Dev'],
    configs: [{ path: 'Prod' }, { path: 'Dev' }] as GroupConfig[],
    hosts: [makeHost('h1', 'Prod'), makeHost('h2', 'Prod/Web'), makeHost('h3', 'Dev')],
    managedSources: [],
    snippets: [],
  };

  let deleteGroups: ((paths: Iterable<string>, deleteHosts?: boolean) => Promise<void>) | undefined;
  let renderer: ReactTestRenderer | null = null;
  let commitCount = 0;
  let supersedeCount = 0;

  const Probe = () => {
    deleteGroups = useVaultGroupDeletion({
      customGroups: persisted.groups,
      hosts: persisted.hosts,
      groupConfigs: persisted.configs,
      managedSources: persisted.managedSources,
      onReadPersistedHosts: async () => persisted.hosts,
      onReadPersistedManagedSources: () => persisted.managedSources,
      onCommitVaultGroupMutation: async (mutate) => {
        commitCount += 1;
        const result = mutate({
          groups: persisted.groups,
          configs: persisted.configs,
          hosts: persisted.hosts,
          managedSources: persisted.managedSources,
          snippets: persisted.snippets as never[],
        });
        if (!result.ok) return result;
        persisted = {
          groups: result.state.groups,
          configs: result.state.configs,
          hosts: result.state.hosts,
          managedSources: result.state.managedSources,
          snippets: persisted.snippets,
        };
        return result;
      },
    }) as typeof deleteGroups;
    return null;
  };

  try {
    await act(async () => {
      renderer = create(React.createElement(Probe));
    });
    await act(async () => {
      await deleteGroups?.(['Prod'], true);
    });
  } finally {
    await act(async () => {
      renderer?.unmount();
    });
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = previousActEnvironment;
  }

  assert.equal(commitCount, 1, 'a single commit must carry the whole mutation');
  assert.equal(supersedeCount, 0);
  assert.deepEqual(persisted.groups, ['Dev'], 'Prod tree must be gone');
  assert.deepEqual(
    persisted.hosts.map((host) => host.id),
    ['h3'],
    'deleteHosts=true must remove every host under the group tree',
  );
});

test('buildVaultGroupDeletion with deleteHosts removes nested hosts and keeps siblings', () => {
  const deletion = buildVaultGroupDeletion({
    selectedPaths: ['Prod'],
    deleteHosts: true,
    customGroups: ['Prod', 'Prod/Web', 'Dev'],
    hosts: [makeHost('h1', 'Prod'), makeHost('h2', 'Prod/Web'), makeHost('h3', 'Dev')],
    groupConfigs: [{ path: 'Prod' }] as GroupConfig[],
    managedSources: [],
  });

  assert.deepEqual(deletion.selectedRoots, ['Prod']);
  assert.deepEqual(deletion.customGroups, ['Dev']);
  assert.deepEqual(deletion.hosts.map((host) => host.id), ['h3']);
});

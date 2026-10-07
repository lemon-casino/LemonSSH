/**
 * Regression test: an unrelated vault re-render must not cancel an in-flight
 * group-deletion transaction.
 *
 * `useVaultState` keeps `hostsRef`/`customGroupsRef`/… as the live snapshot a
 * locked transaction validates against. When those refs were assigned in the
 * render body, a re-render triggered while the transaction awaited the vault
 * lock rewound them to the pre-transaction state, `commitVaultGroupMutation`
 * reported "superseded", and `useVaultGroupDeletion` retried forever — the
 * group-delete dialog closed with no visible change.
 *
 * This test drives the real hook with a commit implementation that mirrors the
 * supersede contract, and injects a re-render between `onReadPersistedHosts`
 * and the commit. The deletion must still land.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import { act, create, type ReactTestRenderer } from 'react-test-renderer';

import type { GroupConfig, Host, ManagedSource } from '../../domain/models.ts';
import { useVaultGroupDeletion } from './useVaultGroupDeletion.ts';

type VaultState = {
  groups: string[];
  configs: GroupConfig[];
  hosts: Host[];
  managedSources: ManagedSource[];
  snippets: never[];
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

test('an unrelated re-render during the vault lock does not supersede the deletion', async () => {
  const actEnvironment = globalThis as typeof globalThis & {
    IS_REACT_ACT_ENVIRONMENT?: boolean;
  };
  const previousActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT;
  actEnvironment.IS_REACT_ACT_ENVIRONMENT = true;

  let persisted: VaultState = {
    groups: ['Prod', 'Prod/Web', 'Dev'],
    configs: [{ path: 'Prod' }, { path: 'Dev' }] as GroupConfig[],
    hosts: [makeHost('h1', 'Prod'), makeHost('h2', 'Prod/Web'), makeHost('h3', 'Dev')],
    managedSources: [],
    snippets: [],
  };

  let deleteGroups: ((paths: Iterable<string>, deleteHosts?: boolean) => Promise<void>) | undefined;
  let renderer: ReactTestRenderer | null = null;
  let triggerRerender: (() => void) | null = null;
  let commitCount = 0;
  let supersedeCount = 0;
  let injected = false;

  const Probe = () => {
    const [, forceRender] = React.useState(0);
    triggerRerender = () => forceRender((value) => value + 1);

    deleteGroups = useVaultGroupDeletion({
      customGroups: persisted.groups,
      hosts: persisted.hosts,
      groupConfigs: persisted.configs,
      managedSources: persisted.managedSources,
      onReadPersistedHosts: async () => {
        // Simulate an unrelated vault write landing while the transaction
        // waits for the lock: a host is renamed elsewhere (identity changes,
        // membership does not), which re-renders VaultPublisher.
        if (!injected) {
          injected = true;
          persisted = {
            ...persisted,
            hosts: persisted.hosts.map((host) =>
              host.id === 'h3' ? { ...host, label: 'h3-renamed' } : host,
            ),
          };
          triggerRerender?.();
        }
        return persisted.hosts;
      },
      onReadPersistedManagedSources: () => persisted.managedSources,
      onCommitVaultGroupMutation: async (mutate) => {
        commitCount += 1;
        const result = mutate({
          groups: persisted.groups,
          configs: persisted.configs,
          hosts: persisted.hosts,
          managedSources: persisted.managedSources,
          snippets: persisted.snippets,
        });
        if (!result.ok) {
          supersedeCount += 1;
          return result;
        }
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

  assert.ok(injected, 'the harness must have injected the concurrent write');
  assert.equal(supersedeCount, 0, 'a concurrent unrelated write must not supersede');
  assert.ok(commitCount >= 1, 'the commit must run');
  assert.deepEqual(persisted.groups, ['Dev'], 'the Prod tree must be deleted');
  assert.deepEqual(
    persisted.hosts.map((host) => host.id),
    ['h3'],
    'every host under Prod must be removed with deleteHosts=true',
  );
  assert.equal(persisted.hosts[0]?.label, 'h3-renamed', 'the concurrent rename must survive');
});

/**
 * Reproduction harness for "workbench tree group delete does nothing".
 *
 * Path under test (workbench layout):
 *   WorkbenchSessionTree group row context menu -> menuActions.onDeleteGroup
 *     -> vaultHostTreeActionsStore -> startInlineDeleteGroup
 *     -> hostTreeInlineGroupDeleteStore.open(groupPath)
 *     -> HostTreeGroupDeleteDialog confirm
 *     -> deleteGroupPath(path, isManaged || deleteHosts)
 *     -> useVaultGroupDeletion -> buildVaultGroupDeletion -> commit
 *
 * The contract asserted here is deliberately source-level: the workbench tree
 * must publish `onDeleteGroup` for group rows, and that action must open the
 * delete dialog with the clicked group path. If the row equality / memo layer
 * stops the delete action from reaching the store, these assertions fail.
 */
import assert from 'node:assert/strict';
import test from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const WORKBENCH_TREE = new URL('../../components/workbench/WorkbenchSessionTree.tsx', import.meta.url);
const INLINE_ACTIONS = new URL('./useHostTreeInlineGroupActions.ts', import.meta.url);
const DELETE_STORE = new URL('../../application/state/hostTreeInlineGroupDeleteStore.ts', import.meta.url);
const DELETE_DIALOG = new URL('../../components/host/HostTreeGroupDeleteDialog.tsx', import.meta.url);

const readSource = (url: URL) => readFile(fileURLToPath(url), 'utf8');

test('workbench tree publishes a group delete action for group rows', async () => {
  const source = await readSource(WORKBENCH_TREE);

  // Group rows must wire the delete menu item to the shared action.
  assert.match(
    source,
    /node\.type === "group" && onDeleteGroup[\s\S]{0,200}onClick=\{\(\) => onDeleteGroup\(node\.id\)\}/,
    'workbench group row must call onDeleteGroup(node.id)',
  );

  // The layer must forward the store-provided action into the tree.
  assert.match(
    source,
    /onDeleteGroup=\{menuActions\?\.onDeleteGroup\}/,
    'workbench tree must receive onDeleteGroup from vaultHostTreeActionsStore',
  );
});

test('group delete action opens the dialog store with the group path', async () => {
  const [inlineActions, deleteStore] = await Promise.all([
    readSource(INLINE_ACTIONS),
    readSource(DELETE_STORE),
  ]);

  assert.match(
    inlineActions,
    /const startInlineDeleteGroup = useCallback\(\(groupPath: string\) => \{\s*hostTreeInlineGroupDeleteStore\.open\(groupPath\);\s*\}, \[\]\);/,
    'startInlineDeleteGroup must open the delete store with the clicked path',
  );

  // The store must notify subscribers so the dialog re-renders on open.
  assert.match(deleteStore, /getTargetPath = \(\) => this\.targetPath;/);
  assert.match(
    deleteStore,
    /open = \(groupPath: string\) => \{\s*this\.targetPath = groupPath;\s*this\.listeners\.forEach\(\(listener\) => listener\(\)\);\s*\};/,
    'store.open must flush listeners',
  );
});

test('delete dialog forwards the deleteHosts checkbox value on confirm', async () => {
  const source = await readSource(DELETE_DIALOG);

  assert.match(
    source,
    /onConfirmDelete\(targetPath, isManaged \|\| deleteHosts\)/,
    'confirm must pass the checkbox (deleteHosts) through to onConfirmDelete',
  );
});

import assert from 'node:assert/strict';
import test from 'node:test';
import { createNativeFileActions } from './nativeFileActions';
for (const outcome of ['complete', 'cancel', 'failure', 'invalid', 'returned-error'] as const) {
    test(`temp download ${outcome} validates completed files and cleans unsuccessful files`, async () => {
        const files = new Set<string>();
        const actions = createNativeFileActions({
            TempFilePath: async () => '/managed/a.txt',
            ValidateTempFile: async (p) => { if (!files.has(p))
                throw new Error('invalid download'); },
            DeleteTempFile: async (p) => { files.delete(p); },
        }, undefined, async (options) => {
            if (outcome !== 'invalid')
                files.add(options.targetPath);
            if (outcome === 'failure')
                throw new Error('transfer failed');
            return { transferId: options.transferId, cancelled: outcome === 'cancel', error: outcome === 'returned-error' ? 'partial download failed' : undefined };
        }, 'win32');
        const run = () => actions.downloadSftpToTempWithProgress('sftp', '/a.txt', 'a.txt', undefined, 'task');
        if (outcome === 'failure' || outcome === 'invalid' || outcome === 'returned-error')
            await assert.rejects(run);
        else
            assert.deepEqual(await run(), { localPath: outcome === 'cancel' ? '' : '/managed/a.txt', cancelled: outcome === 'cancel' });
        assert.equal(files.size, outcome === 'complete' ? 1 : 0);
    });
}
test('native dialogs receive filename and platform application filters', async () => {
    const options: Record<string, unknown>[] = [];
    const actions = createNativeFileActions(undefined, {
        OpenFile: async (value) => { options.push(value); return 'C:/Apps/editor.exe'; },
        SaveFile: async (value) => { options.push(value); return ''; },
    }, async () => ({ transferId: 'unused' }), 'win32');
    assert.deepEqual(await actions.selectApplication(), { path: 'C:/Apps/editor.exe', name: 'editor.exe' });
    await actions.showSaveDialog('C:/Downloads/report.txt');
    assert.deepEqual(options[0].Filters, [{ DisplayName: 'Applications', Pattern: '*.exe' }]);
    assert.equal(options[1].Filename, 'report.txt');
    assert.equal(options[1].Directory, 'C:/Downloads');
    await assert.rejects(() => actions.openWithApplication('/a', '/app'), /unavailable/);
    assert.equal((await actions.openWithSystemDefault('/a')).success, false);
});

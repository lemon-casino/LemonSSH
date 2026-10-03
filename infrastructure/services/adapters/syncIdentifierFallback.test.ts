import assert from 'node:assert/strict';
import test from 'node:test';

import { downloadSyncGist, findSyncGist } from './GitHubAdapter.ts';
import { findSyncFile as findGoogleDriveSyncFile } from './GoogleDriveAdapter.ts';
import { findSyncFile as findOneDriveSyncFile } from './OneDriveAdapter.ts';
import { SYNC_CONSTANTS } from '../../../domain/sync.ts';

// compat#5: the frontend read surface must accept cloud artifacts created by
// pre-rename builds (legacy file name / gist description) while writes keep
// using the new names. The bridge is unavailable under node:test, so these
// tests drive the direct fetch paths.

type FetchCall = { url: string };

function installFetchMock(
  handler: (url: string, init?: RequestInit) => Promise<Response> | Response,
): { calls: FetchCall[]; restore: () => void } {
  const calls: FetchCall[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    calls.push({ url });
    return handler(url, init);
  }) as typeof fetch;
  return {
    calls,
    restore: () => {
      globalThis.fetch = original;
    },
  };
}

test('findSyncGist matches a gist created by a pre-rename build', async () => {
  const { restore } = installFetchMock(() => new Response(JSON.stringify([
    {
      id: 'unrelated',
      description: 'some other gist',
      files: { 'notes.txt': { filename: 'notes.txt', content: 'x' } },
    },
    {
      id: 'gist-legacy',
      description: SYNC_CONSTANTS.LEGACY_GIST_DESCRIPTION,
      files: {
        [SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME]: {
          filename: SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME,
          content: '{"meta":{"version":1}}',
        },
      },
    },
  ]), { status: 200 }));
  try {
    const gistId = await findSyncGist('token-1');
    assert.equal(gistId, 'gist-legacy');
  } finally {
    restore();
  }
});

test('findSyncGist still matches gists written with the new identifiers', async () => {
  const { restore } = installFetchMock(() => new Response(JSON.stringify([
    {
      id: 'gist-new',
      description: SYNC_CONSTANTS.GIST_DESCRIPTION,
      files: {
        [SYNC_CONSTANTS.SYNC_FILE_NAME]: {
          filename: SYNC_CONSTANTS.SYNC_FILE_NAME,
          content: '{"meta":{"version":2}}',
        },
      },
    },
  ]), { status: 200 }));
  try {
    assert.equal(await findSyncGist('token-1'), 'gist-new');
  } finally {
    restore();
  }
});

test('downloadSyncGist falls back to the legacy file name inside the gist', async () => {
  const { restore } = installFetchMock((url) => {
    if (url.includes('/gists/gist-legacy')) {
      return new Response(JSON.stringify({
        id: 'gist-legacy',
        description: SYNC_CONSTANTS.LEGACY_GIST_DESCRIPTION,
        files: {
          // Only the pre-rename file name exists in this gist.
          [SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME]: {
            filename: SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME,
            content: '{"meta":{"version":3},"payload":"ENC"}',
          },
        },
      }), { status: 200 });
    }
    return new Response('unexpected', { status: 404 });
  });
  try {
    const syncedFile = await downloadSyncGist('token-1', 'gist-legacy');
    assert.ok(syncedFile);
    assert.equal(syncedFile?.meta?.version, 3);
  } finally {
    restore();
  }
});

test('Google Drive findSyncFile falls back to the legacy file name', async () => {
  const { calls, restore } = installFetchMock((url) => {
    if (url.includes(encodeURIComponent(SYNC_CONSTANTS.SYNC_FILE_NAME))
      || url.includes(SYNC_CONSTANTS.SYNC_FILE_NAME)) {
      return new Response(JSON.stringify({ files: [] }), { status: 200 });
    }
    if (url.includes(encodeURIComponent(SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME))
      || url.includes(SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME)) {
      return new Response(JSON.stringify({
        files: [{ id: 'drive-legacy-id', name: SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME }],
      }), { status: 200 });
    }
    return new Response(JSON.stringify({ files: [] }), { status: 200 });
  });
  try {
    const fileId = await findGoogleDriveSyncFile('token-1');
    assert.equal(fileId, 'drive-legacy-id');
    assert.equal(calls.length, 2, 'expected one query per candidate name');
    assert.ok(calls[0].url.includes(SYNC_CONSTANTS.SYNC_FILE_NAME));
    assert.ok(calls[1].url.includes(SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME));
  } finally {
    restore();
  }
});

test('OneDrive findSyncFile falls back to the legacy path', async () => {
  const { calls, restore } = installFetchMock((url) => {
    if (url.includes(SYNC_CONSTANTS.SYNC_FILE_NAME)) {
      return new Response('not found', { status: 404 });
    }
    if (url.includes(SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME)) {
      return new Response(JSON.stringify({ id: 'onedrive-legacy-id' }), { status: 200 });
    }
    return new Response('not found', { status: 404 });
  });
  try {
    const fileId = await findOneDriveSyncFile('token-1');
    assert.equal(fileId, 'onedrive-legacy-id');
    assert.ok(calls.some(call => call.url.includes(SYNC_CONSTANTS.LEGACY_SYNC_FILE_NAME)));
  } finally {
    restore();
  }
});

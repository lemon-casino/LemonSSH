import assert from 'node:assert/strict';
import test from 'node:test';

import {
  applyImportedKeyGroupToDraft,
  classifyImportedKeyContent,
  groupImportedKeyFiles,
  sshKeyFileStem,
} from './sshKeyImport.ts';

const PRIVATE = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
-----END OPENSSH PRIVATE KEY-----`;
const PUBLIC = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIprivatepair user@host';
const CERT = 'ssh-ed25519-cert-v01@openssh.com AAAAHHNzaC1lZDI1NTE5LWNlcnQtdjAx user@host';
const RSA_PRIVATE = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA
-----END RSA PRIVATE KEY-----`;

test('classifies OpenSSH private, public, and certificate material', () => {
  assert.equal(classifyImportedKeyContent(PRIVATE).role, 'private');
  assert.equal(classifyImportedKeyContent(PRIVATE).type, 'ED25519');
  assert.equal(classifyImportedKeyContent(PUBLIC).role, 'public');
  assert.equal(classifyImportedKeyContent(CERT).role, 'certificate');
  assert.equal(classifyImportedKeyContent(RSA_PRIVATE).type, 'RSA');
});

test('stems pair id_ed25519 with id_ed25519.pub', () => {
  assert.equal(sshKeyFileStem('id_ed25519'), 'id_ed25519');
  assert.equal(sshKeyFileStem('id_ed25519.pub'), 'id_ed25519');
  assert.equal(sshKeyFileStem('id_ed25519-cert.pub'), 'id_ed25519');
  assert.equal(sshKeyFileStem('C:\\\\Users\\\\a\\\\.ssh\\\\host_key.pub'), 'host_key');
});

test('groups a private/public/cert trio selected together into one key', () => {
  const groups = groupImportedKeyFiles([
    { name: 'id_ed25519.pub', content: PUBLIC },
    { name: 'id_ed25519', content: PRIVATE },
    { name: 'id_ed25519-cert.pub', content: CERT },
  ]);
  assert.equal(groups.length, 1);
  assert.equal(groups[0]?.label, 'id_ed25519');
  assert.equal(groups[0]?.privateKey, PRIVATE);
  assert.equal(groups[0]?.publicKey, PUBLIC);
  assert.equal(groups[0]?.certificate, CERT);
  assert.equal(groups[0]?.type, 'ED25519');
});

test('unpaired public keys stay public-only and do not become private keys', () => {
  const groups = groupImportedKeyFiles([{ name: 'guest.pub', content: PUBLIC }]);
  assert.equal(groups.length, 1);
  assert.equal(groups[0]?.publicKey, PUBLIC);
  assert.equal(groups[0]?.privateKey, undefined);
});

test('a lone private and a lone public still pair when selected together', () => {
  const groups = groupImportedKeyFiles([
    { name: 'alpha', content: PRIVATE },
    { name: 'beta.pub', content: PUBLIC },
  ]);
  assert.equal(groups.length, 1);
  assert.equal(groups[0]?.privateKey, PRIVATE);
  assert.equal(groups[0]?.publicKey, PUBLIC);
});

test('two complete pairs stay two groups', () => {
  const groups = groupImportedKeyFiles([
    { name: 'alpha', content: PRIVATE },
    { name: 'alpha.pub', content: PUBLIC },
    { name: 'beta', content: RSA_PRIVATE },
    { name: 'beta.pub', content: 'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ beta' },
  ]);
  assert.equal(groups.length, 2);
  assert.deepEqual(groups.map((group) => group.label).sort(), ['alpha', 'beta']);
});

test('draft merge fills empty private/public fields from the grouped files', () => {
  const next = applyImportedKeyGroupToDraft({ label: '' }, {
    label: 'id_ed25519',
    type: 'ED25519',
    privateKey: PRIVATE,
    publicKey: PUBLIC,
  });
  assert.equal(next.label, 'id_ed25519');
  assert.equal(next.privateKey, PRIVATE);
  assert.equal(next.publicKey, PUBLIC);
});

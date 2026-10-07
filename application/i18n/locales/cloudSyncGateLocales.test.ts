import test from 'node:test';
import assert from 'node:assert/strict';

import en from '../locales/en.ts';
import ru from '../locales/ru.ts';
import es from '../locales/es.ts';
import zhCN from '../locales/zh-CN.ts';
import zhTW from '../locales/zh-TW.ts';

const GATE_KEYS = [
  'cloudSync.gate.enableVault',
  'cloudSync.gate.enabledToast',
  'cloudSync.header.vaultReady',
  'cloudSync.header.preparingVault',
  'cloudSync.unlock.failed',
  'cloudSync.unlock.readyToast',
  'cloudSync.changeKey.desc',
  'cloudSync.clearLocal.dialog.title',
] as const;

test('cloud sync vault CTA follows each bundled language instead of leftover English Vault', () => {
  const locales = { en, ru, es, zhCN, zhTW } as const;
  for (const [locale, messages] of Object.entries(locales)) {
    for (const key of GATE_KEYS) {
      assert.equal(typeof messages[key], 'string', `${locale} is missing ${key}`);
      assert.notEqual(messages[key], '', `${locale} has empty ${key}`);
    }
  }

  assert.match(en['cloudSync.gate.enableVault'], /Vault/i);
  assert.match(es['cloudSync.gate.enableVault'], /bóveda/i);
  assert.match(ru['cloudSync.gate.enableVault'], /хранилищ/i);

  for (const key of GATE_KEYS) {
    assert.equal(
      zhCN[key].includes('Vault'),
      false,
      `zh-CN ${key} must use 保险库, got ${zhCN[key]}`,
    );
    assert.match(zhCN[key], /保险库/, `zh-CN ${key} must name 保险库`);
    assert.equal(
      zhTW[key].includes('Vault'),
      false,
      `zh-TW ${key} must use 保險庫, got ${zhTW[key]}`,
    );
    assert.match(zhTW[key], /保險庫/, `zh-TW ${key} must name 保險庫`);
  }
});

test('gate warning matches current reset rules: keep local vault, save client IDs, reset clears them', () => {
  const locales = { en, ru, es, zhCN, zhTW } as const;
  const staleDenial = [
    'There is no password reset',
    'No hay restablecimiento de contraseña',
    'Сброс пароля невозможен',
    '没有密码重置功能',
    '沒有密碼重設功能',
  ];
  for (const [locale, messages] of Object.entries(locales)) {
    const warning = messages['cloudSync.gate.warning'];
    assert.equal(typeof warning, 'string', `${locale} is missing cloudSync.gate.warning`);
    for (const stale of staleDenial) {
      assert.equal(
        warning.includes(stale),
        false,
        `${locale} still denies password reset: ${warning}`,
      );
    }
  }

  assert.match(zhCN['cloudSync.gate.warning'], /重置初始化|重置云同步/);
  assert.match(zhCN['cloudSync.gate.warning'], /保险库/);
  assert.match(zhCN['cloudSync.gate.warning'], /client ID/i);
  assert.match(zhTW['cloudSync.gate.warning'], /重設初始化|重設雲端同步/);
  assert.match(zhTW['cloudSync.gate.warning'], /保險庫/);
  assert.match(zhTW['cloudSync.gate.warning'], /client ID/i);
  assert.match(en['cloudSync.gate.warning'], /reset/i);
  assert.match(en['cloudSync.gate.warning'], /client ID/i);
  assert.match(en['cloudSync.gate.warning'], /vault/i);
  assert.match(es['cloudSync.gate.warning'], /client ID|ID de cliente/i);
  assert.match(ru['cloudSync.gate.warning'], /client ID/i);
});

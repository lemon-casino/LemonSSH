import test from 'node:test';
import assert from 'node:assert/strict';

import en from '../locales/en.ts';
import ru from '../locales/ru.ts';
import es from '../locales/es.ts';
import zhCN from '../locales/zh-CN.ts';
import zhTW from '../locales/zh-TW.ts';

test('Google client-ID tooltip lists desktop app, secret, Drive API, scopes and test users', () => {
  const locales = { en, ru, es, zhCN, zhTW } as const;
  for (const [locale, messages] of Object.entries(locales)) {
    const guide = messages['cloudSync.oauth.guide.google'];
    assert.equal(typeof guide, 'string', `${locale} missing cloudSync.oauth.guide.google`);
    assert.match(guide, /drive\.appdata/, `${locale} must name the drive.appdata scope`);
  }

  assert.match(zhCN['cloudSync.oauth.guide.google'], /桌面应用/);
  assert.match(zhCN['cloudSync.oauth.guide.google'], /client_secret|客户端密钥/);
  assert.match(zhCN['cloudSync.oauth.guide.google'], /Drive API/);
  assert.match(zhCN['cloudSync.oauth.guide.google'], /测试用户/);
  assert.match(zhTW['cloudSync.oauth.guide.google'], /桌面應用/);
  assert.match(zhTW['cloudSync.oauth.guide.google'], /Drive API/);
  assert.match(zhTW['cloudSync.oauth.guide.google'], /測試使用者|測試用戶/);
  assert.match(en['cloudSync.oauth.guide.google'], /Desktop app/i);
  assert.match(en['cloudSync.oauth.guide.google'], /client_secret/i);
  assert.match(en['cloudSync.oauth.guide.google'], /Drive API/);
  assert.match(en['cloudSync.oauth.guide.google'], /test user/i);
});

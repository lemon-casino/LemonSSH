import assert from 'node:assert/strict';
import test from 'node:test';

import en from './en.ts';
import ru from './ru.ts';
import es from './es.ts';
import zhCN from './zh-CN.ts';
import zhTW from './zh-TW.ts';

const AGENT_APPROVAL_KEYS = [
  'ai.agentApproval.title',
] as const;

test('Go agent interaction approval UI is localized in every supported locale', () => {
  for (const [name, messages] of Object.entries({ en, es, 'zh-CN': zhCN, 'zh-TW': zhTW, ru })) {
    const missing = AGENT_APPROVAL_KEYS.filter(key => !messages[key]);
    assert.deepEqual(missing, [], `${name} is missing agent approval labels`);
  }
});

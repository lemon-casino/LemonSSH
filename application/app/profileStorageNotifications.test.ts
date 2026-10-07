import assert from 'node:assert/strict';
import { test } from 'node:test';
import { subscribeProfileStorageErrors } from './profileStorageNotifications';
import { HOST_PROFILE_ERROR_EVENT } from '../../infrastructure/persistence/hostStorageAdapter';

test('profile failure notification uses localized generic text, ignores secret details, and unsubscribes', () => {
  const target = new EventTarget();
  const shown: string[] = [];
  const dispose = subscribeProfileStorageErrors(target, () => shown.push('Localized save failure'));
  const event = new Event(HOST_PROFILE_ERROR_EVENT);
  Object.defineProperty(event, 'detail', { get() { throw new Error('secret detail must never be inspected'); } });
  target.dispatchEvent(event);
  assert.deepEqual(shown, ['Localized save failure']);
  // A burst from a failed disk or conflict must not flood the toast owner.
  target.dispatchEvent(event);
  assert.equal(shown.length, 1);
  dispose();
  target.dispatchEvent(event);
  assert.equal(shown.length, 1);
});

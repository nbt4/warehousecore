import assert from 'node:assert/strict';
import test from 'node:test';
import { apiErrorMessage } from '../src/lib/api-errors.js';

test('server validation errors take precedence over transport errors', () => {
  assert.equal(apiErrorMessage({
    response: { data: { error: 'Invalid status' } },
    message: 'Request failed with status code 400',
  }), 'Invalid status');
});

test('callers can preserve message-first server responses', () => {
  assert.equal(apiErrorMessage({
    response: { data: { message: 'Restore first', error: 'Conflict' } },
  }, 'Delete failed', ['message', 'error']), 'Restore first');
});

test('malformed responses cannot become UI messages or throw', () => {
  for (const error of [null, undefined, 42, {}, { response: null },
    { response: { data: null } }, { response: { data: { error: 42 } } },
    { response: { data: { error: '' } } }]) {
    assert.equal(apiErrorMessage(error, 'Save failed'), 'Save failed');
  }
});

test('explicit fallbacks and ordinary error messages keep their precedence', () => {
  assert.equal(apiErrorMessage(new Error('Network error')), 'Network error');
  assert.equal(apiErrorMessage(new Error('Network error'), 'Scan failed'), 'Scan failed');
  assert.equal(apiErrorMessage('Connection closed'), 'Connection closed');
});

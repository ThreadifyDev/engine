import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const output = await build({
  stdin: {
    contents: `import React from 'react';
      import {renderToStaticMarkup} from 'react-dom/server';
      import {ValidationResultsView} from './app/components/thread/ValidationResultsView';
      export const render = props => renderToStaticMarkup(React.createElement(ValidationResultsView, props));`,
    resolveDir: fileURLToPath(new URL('..', import.meta.url)),
  },
  bundle: true, write: false, format: 'cjs', packages: 'external', platform: 'node', jsx: 'automatic',
});
const compiled = {exports: {}};
new Function('require', 'module', 'exports', output.outputFiles[0].text)(createRequire(import.meta.url), compiled, compiled.exports);
const {render} = compiled.exports;
const base = {notificationsLoading: false, severityFilter: new Set(['critical', 'unclassified']), onSeverityFilterChange() {}, onNotificationClick() {}};
const notification = {notificationId: 'grouped-approval', stepName: 'refund', severity: '', message: 'Refund completed with two approval violations', source: 'rule', timestamp: new Date().toISOString()};

test('a preserved grouped finding without severity remains visible', () => {
  const html = render({...base, notifications: [notification]});
  assert.match(html, /Refund completed with two approval violations/);
  assert.match(html, /unclassified/);
  assert.doesNotMatch(html, /All validations passed/);
});

test('hiding a severity explains the empty filtered view', () => {
  const html = render({...base, notifications: [notification], severityFilter: new Set(['critical'])});
  assert.match(html, /No notifications match the selected severities/);
  assert.doesNotMatch(html, /Refund completed with two approval violations/);
});

test('an empty response does not assert successful validation', () => {
  const html = render({...base, notifications: []});
  assert.match(html, /No validation notifications were returned/);
  assert.doesNotMatch(html, /All validations passed/);
});

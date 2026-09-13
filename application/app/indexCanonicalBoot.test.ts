import assert from 'node:assert/strict';
import test from 'node:test';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';
import ts from 'typescript';

async function bootFixture() {
  const source = await readFile(new URL('../../index.tsx', import.meta.url), 'utf8');
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 } }).outputText;
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const hydrateReady = new Promise<void>((yes, no) => { resolve = yes; reject = no; });
  const imported: string[] = [];
  const renders: unknown[] = [];
  const handlers = new Map<string, () => void>();
  let splashRemoved = false;
  const component = () => null;
  const context = {
    exports: {}, console: { error() {} },
    document: { getElementById: (id: string) => id === 'root' ? {} : { remove: () => { splashRemoved = true; } }, documentElement: { classList: { add() {}, remove() {} } } },
    window: { location: { hash: '', reload() {} }, addEventListener: (name: string, fn: () => void) => handlers.set(name, fn) },
    require: (name: string) => {
      if (name.endsWith('.css')) return {};
      if (name === 'react') return { StrictMode: 'strict', Suspense: 'suspense', lazy: (load: unknown) => ({ load }) };
      if (name === 'react/jsx-runtime') return { jsx: (type: unknown, props: unknown) => ({ type, props }), jsxs: (type: unknown, props: unknown) => ({ type, props }) };
      if (name === 'react-dom/client') return { default: { createRoot: () => ({ render: (value: unknown) => renders.push(value) }) } };
      if (name.endsWith('/bootstrap')) return { hydrateReady, installRuntimeClient() {} };
      imported.push(name);
      return { default: component, AppLockGate: component };
    },
  };
  vm.runInNewContext(code, context);
  const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };
  return { context, imported, renders, handlers, resolve, reject, flush, splashRemoved: () => splashRemoved };
}

test('entry delays all component evaluation and hash routing until canonical hydration', async () => {
  const boot = await bootFixture();
  assert.deepEqual(boot.imported, [], 'eager component imports can freeze stale localStorage singleton state');
  boot.context.window.location.hash = '#/settings';
  boot.handlers.get('hashchange')?.();
  assert.equal(boot.renders.length, 0);
  boot.resolve();
  await boot.flush();
  assert.ok(boot.imported.includes('./components/AppLockGate'));
  assert.equal(boot.renders.length, 1);
  assert.ok(boot.handlers.has('hashchange'));
});

test('failed hydration never evaluates components or permits hashchange boot and retains retry alert', async () => {
  const boot = await bootFixture();
  boot.reject(new Error('unavailable'));
  await boot.flush();
  assert.deepEqual(boot.imported, []);
  assert.equal(boot.handlers.has('hashchange'), false);
  assert.equal(boot.splashRemoved(), true);
  assert.equal((boot.renders[0] as { props: { role: string } }).props.role, 'alert');
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { JSDOM } from 'jsdom';
import type { SftpFileEntry } from '../../domain/models/sftp';

test('permission save waits, keeps dialog open on rejection, and allows retry', async () => {
    const dom = new JSDOM('<html><body><div id="root"></div></body></html>', { url: 'http://localhost', pretendToBeVisual: true });
    const old = new Map<string, PropertyDescriptor | undefined>();
    const windowRecord = dom.window as unknown as Record<string, unknown>;
    for (const name of ['window', 'document', 'navigator', 'HTMLElement', 'HTMLInputElement', 'Element', 'Node', 'NodeFilter', 'MutationObserver', 'CustomEvent', 'Event', 'localStorage', 'sessionStorage']) {
        old.set(name, Object.getOwnPropertyDescriptor(globalThis, name));
        Object.defineProperty(globalThis, name, { configurable: true, writable: true, value: name === 'window' ? dom.window : windowRecord[name] });
    }
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true, getComputedStyle: dom.window.getComputedStyle.bind(dom.window) });
    const { default: React, act } = await import('react');
    const { createRoot } = await import('react-dom/client');
    const { SftpPermissionsDialog } = await import('./SftpPermissionsDialog');
    const root = createRoot(dom.window.document.getElementById('root')!);
    let reject!: (error: Error) => void;
    let closed = false;
    let save = () => new Promise<void>((_, r) => { reject = r; });
    try {
        await act(async () => root.render(React.createElement(SftpPermissionsDialog, { open: true, onOpenChange: open => { closed = !open; }, file: { name: 'a.txt', permissions: '644' } as unknown as SftpFileEntry, onSave: () => save() })));
        const apply = () => [...dom.window.document.querySelectorAll('button')].find(b => /Apply|应用|common.apply/.test(b.textContent ?? ''))!;
        await act(async () => { apply().click(); });
        assert.equal(closed, false, 'must not close while saving');
        assert.equal(apply().disabled, true);
        await act(async () => reject(new Error('Permission denied')));
        assert.equal(closed, false);
        assert.match(dom.window.document.querySelector('[role="alert"]')?.textContent ?? '', /Permission denied/);
        save = async () => { };
        await act(async () => { apply().click(); });
        assert.equal(closed, true);
    }
    finally {
        await act(async () => root.unmount());
        await new Promise(resolve => setTimeout(resolve, 10));
        dom.window.close();
        for (const [name, descriptor] of old) {
            if (descriptor)
                Object.defineProperty(globalThis, name, descriptor);
            else
                Reflect.deleteProperty(globalThis, name);
        }
    }
});

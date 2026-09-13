import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import assert from 'node:assert/strict';
import test from 'node:test';
import { createDomRenderer, dispatchDomEvent, installDomEnvironment } from '../test-support/renderReactDom';
import { I18nProvider } from '../../application/i18n/I18nProvider';
import { DeclarativePluginHost, PluginInstallError } from './DeclarativePluginHost';

test('host renders typed controls, escaped content, and never restores secrets', () => {
 const html = renderToStaticMarkup(<DeclarativePluginHost pluginId="example" schema={{settings:[{id:'enabled',type:'boolean',label:'Enabled'},{id:'token',type:'password',label:'Token'}],views:[{id:'status',type:'card',title:'<script>bad</script>',bindings:['status']}]}} values={{enabled:true,token:'do-not-expose'}} data={{status:'Ready'}} onSettingChange={async () => {}} />);
 assert.ok(html.includes('type="checkbox"'));
 assert.ok(html.includes('type="password"'));
 assert.ok(!html.includes('do-not-expose'));
 assert.ok(!html.includes('<script>'));
 assert.ok(html.includes('Ready'));
});

test('failed setting save restores the committed value and remote reload refreshes controls', async () => {
 const env=installDomEnvironment();const renderer=await createDomRenderer(env.document);
 const props={pluginId:'example',schema:{settings:[{id:'mode',type:'select' as const,label:'Mode',options:['dark','light']}]},data:{},onSettingChange:async()=>{throw Error('Save denied');}};
 try {
  await renderer.render(<DeclarativePluginHost {...props} values={{mode:'dark'}} />);
  const select=renderer.container.querySelector('select')!;
  select.value='light';await dispatchDomEvent(select,new env.window.Event('change',{bubbles:true}));
  assert.equal(select.value,'dark');assert.ok(renderer.container.textContent?.includes('Save denied'));
  await renderer.render(<DeclarativePluginHost {...props} values={{mode:'light'}} />);
  assert.equal(select.value,'light');
 }finally{await renderer.unmount();env.cleanup();}
});

test('legacy incompatibility gives actionable guidance', () => {
 const html = renderToStaticMarkup(<I18nProvider locale="en"><PluginInstallError error="plugin v1 packages are not supported by this runtime" /></I18nProvider>);
 assert.ok(html.includes('role="alert"'));
 assert.ok(html.includes('WASM'));
 assert.ok(html.includes('manifest v2'));
});

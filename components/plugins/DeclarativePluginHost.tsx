import React, { useEffect, useState } from 'react';
import type { DeclarativePluginUI } from '@netcatty/plugin-contract';
import { useI18n } from '../../application/i18n/I18nProvider';

type Value = string | number | boolean;
type Permission = { kind: string; resource: string; mode: string };

export function PluginInstallError({ error }: { error: unknown }) {
  const { t } = useI18n();
  const message = error instanceof Error ? error.message : String(error);
  return (
    <p role="alert" className="text-sm text-destructive">
      {message}
      {/v1|legacy JavaScript\/Node/.test(message) && ` ${t('settings.plugins.legacyGuidance')}`}
    </p>
  );
}

// Bindings are literal data keys. Markup and event handlers belong to the host.
export function DeclarativePluginHost({
  pluginId, schema, values, data, onSettingChange, permissions = [], onGrantPermission,
}: {
  pluginId: string;
  schema: DeclarativePluginUI;
  values: Record<string, Value>;
  data: Record<string, unknown>;
  permissions?: Permission[];
  onGrantPermission?: (permission: Permission, lifetime: 'once' | 'session') => Promise<void>;
  onSettingChange: (id: string, value: Value) => Promise<void>;
}) {
  const { t } = useI18n();
  const [error, setError] = useState<unknown>(null);
  const [pending, setPending] = useState(false);
  const [drafts, setDrafts] = useState<Record<string, Value>>({});
  const [saved, setSaved] = useState(values);
  useEffect(() => { setSaved(values); setDrafts({}); }, [values]);

  const save = async (id: string, value: Value, secret: boolean) => {
    setPending(true);
    setError(null);
    try {
      await onSettingChange(id, value);
      if (!secret) setSaved(current => ({ ...current, [id]: value }));
    } catch (cause) {
      setError(cause);
    } finally {
      setDrafts(current => { const next = { ...current }; delete next[id]; return next; });
      setPending(false);
    }
  };
  const text = (value: unknown) => ['string', 'number', 'boolean'].includes(typeof value) ? String(value) : '';

  return (
    <section aria-label={pluginId} className="space-y-4 overflow-hidden">
      {error != null && <PluginInstallError error={error} />}
      {permissions.map(permission => (
        <div key={JSON.stringify(permission)} className="rounded border border-border p-2 text-sm">
          <span>{permission.kind}: {permission.resource} ({permission.mode})</span>
          {(['once', 'session'] as const).map(lifetime => (
            <button key={lifetime} type="button" disabled={pending || !onGrantPermission}
              className="ml-2 rounded border px-2 py-1"
              onClick={async () => {
                setPending(true); setError(null);
                try { await onGrantPermission?.(permission, lifetime); }
                catch (cause) { setError(cause); }
                finally { setPending(false); }
              }}>
              {t(`settings.plugins.allow.${lifetime}`)}
            </button>
          ))}
        </div>
      ))}
      {(schema.settings ?? []).map(field => {
        const secret = field.type === 'password';
        const value = drafts[field.id] ?? (secret ? '' : saved[field.id] ?? field.default ?? '');
        const update = (next: Value) => setDrafts(current => ({ ...current, [field.id]: next }));
        return (
          <label key={field.id} className="block space-y-1 text-sm">
            <span>{field.label}</span>
            {field.type === 'select' ? (
              <select aria-label={field.label} disabled={pending} value={String(value)}
                onChange={event => { update(event.currentTarget.value); void save(field.id, event.currentTarget.value, false); }}>
                {field.options?.map(option => <option key={option}>{option}</option>)}
              </select>
            ) : (
              <input aria-label={field.label} disabled={pending} required={field.required}
                type={field.type === 'boolean' ? 'checkbox' : field.type}
                checked={field.type === 'boolean' ? value === true || value === 'true' : undefined}
                value={field.type === 'boolean' ? undefined : String(value)}
                onChange={event => {
                  if (field.type === 'boolean') {
                    update(event.currentTarget.checked);
                    void save(field.id, event.currentTarget.checked, false);
                  } else update(event.currentTarget.value);
                }}
                onBlur={event => {
                  if (field.type === 'boolean' || !(field.id in drafts)) return;
                  const next = field.type === 'number' ? event.currentTarget.valueAsNumber : event.currentTarget.value;
                  if (typeof next !== 'number' || Number.isFinite(next)) void save(field.id, next, secret);
                }}
                className="rounded border border-input bg-background px-2 py-1" />
            )}
            {field.description && <span className="block text-xs text-muted-foreground">{field.description}</span>}
          </label>
        );
      })}
      {(schema.views ?? []).map(view => (
        <section key={view.id} aria-label={view.title} className="rounded border border-border p-3">
          <h3>{view.title}</h3>
          {view.type === 'list' ? (
            <table>
              <thead><tr>{view.columns?.map(column => <th key={column}>{column}</th>)}</tr></thead>
              <tbody>{(view.bindings ?? []).map(binding => (
                <tr key={binding}>{(view.columns ?? []).map(column => (
                  <td key={column}>{text((data[binding] as Record<string, unknown> | undefined)?.[column])}</td>
                ))}</tr>
              ))}</tbody>
            </table>
          ) : (
            <dl>{(view.bindings ?? []).map(binding => (
              <React.Fragment key={binding}><dt>{binding}</dt><dd>{text(saved[binding] ?? data[binding])}</dd></React.Fragment>
            ))}</dl>
          )}
        </section>
      ))}
    </section>
  );
}

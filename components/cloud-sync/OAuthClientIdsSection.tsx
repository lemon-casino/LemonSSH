import React from 'react';
import { KeyRound } from 'lucide-react';

import { useI18n } from '../../application/i18n/I18nProvider';
import { Input } from '../ui/input';
import { Label } from '../ui/label';
import { useOAuthClientIds } from '../../application/state/useOAuthClientIds';
import type { OAuthProvider } from '../../infrastructure/services/cloudSync/oauthClientIds';

/**
 * Settings section for the per-provider OAuth client IDs. The IDs are public
 * values, but each user registers their own "desktop app" OAuth client, so
 * they are editable at runtime instead of baked in at build time.
 */
export const OAuthClientIdsSection: React.FC = () => {
  const { t } = useI18n();
  const { ids, setClientId } = useOAuthClientIds();

  const fields: Array<{ provider: OAuthProvider; label: string; placeholder: string }> = [
    { provider: 'github', label: t('cloudSync.oauth.github'), placeholder: 'Iv1.xxxxxxxxxxxxxxxx' },
    { provider: 'google', label: t('cloudSync.oauth.google'), placeholder: 'xxxxxxxx.apps.googleusercontent.com' },
    { provider: 'onedrive', label: t('cloudSync.oauth.onedrive'), placeholder: '00000000-0000-0000-0000-000000000000' },
  ];

  return (
    <section
      data-section="cloud-sync-oauth-client-ids"
      className="rounded-lg border border-border/60 p-4 space-y-3"
    >
      <div className="flex items-center gap-2">
        <KeyRound size={14} className="text-muted-foreground" />
        <span className="text-sm font-medium">{t('cloudSync.oauth.sectionTitle')}</span>
      </div>
      <p className="text-xs text-muted-foreground">{t('cloudSync.oauth.sectionDesc')}</p>
      <div className="grid gap-3 sm:grid-cols-3">
        {fields.map(({ provider, label, placeholder }) => (
          <div key={provider} className="space-y-1">
            <Label className="text-xs" htmlFor={`oauth-client-id-${provider}`}>{label}</Label>
            <Input
              id={`oauth-client-id-${provider}`}
              className="h-8 text-xs font-mono"
              value={ids[provider] ?? ''}
              placeholder={placeholder}
              spellCheck={false}
              autoComplete="off"
              onChange={(event) => setClientId(provider, event.currentTarget.value)}
            />
          </div>
        ))}
      </div>
    </section>
  );
};

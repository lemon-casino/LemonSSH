import React from 'react';
import { CircleHelp, ExternalLink, KeyRound } from 'lucide-react';

import { useI18n } from '../../application/i18n/I18nProvider';
import { Input } from '../ui/input';
import { Label } from '../ui/label';
import { Tooltip, TooltipContent, TooltipTrigger } from '../ui/tooltip';
import { useOAuthClientIds } from '../../application/state/useOAuthClientIds';
import { netcattyBridge } from '../../infrastructure/services/netcattyBridge';
import type { OAuthProvider } from '../../infrastructure/services/cloudSync/oauthClientIds';

/**
 * Settings section for the per-provider OAuth client IDs. The IDs are public
 * values, but each user registers their own "desktop app" OAuth client, so
 * they are editable at runtime instead of baked in at build time.
 */
export const OAuthClientIdsSection: React.FC = () => {
  const { t } = useI18n();
  const { ids, setClientId, googleClientSecret, setGoogleClientSecret } = useOAuthClientIds();

  const openApplyPage = (provider: OAuthProvider) => {
    const bridge = netcattyBridge.get();
    const opener = bridge?.openProviderConsole;
    if (!opener) {
      console.error('Provider console bridge is unavailable');
      return;
    }
    opener(provider).catch((error: unknown) => {
      console.error(`Failed to open the ${provider} console page:`, error);
    });
  };

  const fields: Array<{ provider: OAuthProvider; label: string; placeholder: string }> = [
    { provider: 'github', label: t('cloudSync.oauth.github'), placeholder: 'Iv1.xxxxxxxxxxxxxxxx' },
    { provider: 'google', label: t('cloudSync.oauth.google'), placeholder: 'xxxxxxxx.apps.googleusercontent.com' },
    { provider: 'onedrive', label: t('cloudSync.oauth.onedrive'), placeholder: '00000000-0000-0000-0000-000000000000' },
  ];

  const guideFor = (provider: OAuthProvider): string => t(`cloudSync.oauth.guide.${provider}`);

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
          <div key={provider} className="space-y-2">
            <div className="space-y-1">
              <Label className="flex items-center gap-1 text-xs" htmlFor={`oauth-client-id-${provider}`}>
                {label}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      className="inline-flex items-center text-muted-foreground hover:text-foreground"
                      aria-label={t('cloudSync.oauth.guideTitle')}
                      onClick={() => openApplyPage(provider)}
                    >
                      <CircleHelp size={12} />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-96 whitespace-pre-wrap text-left text-xs">
                    <p className="whitespace-pre-wrap">{guideFor(provider)}</p>
                    <p className="mt-1 flex items-center gap-1 text-[10px] text-muted-foreground">
                      <ExternalLink size={9} />
                      {t('cloudSync.oauth.apply')}
                    </p>
                  </TooltipContent>
                </Tooltip>
              </Label>
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
            {provider === 'google' ? (
              <div className="space-y-1">
                <Label className="text-xs" htmlFor="oauth-client-secret-google">
                  {t('cloudSync.oauth.googleSecret')}
                </Label>
                <Input
                  id="oauth-client-secret-google"
                  type="password"
                  className="h-8 text-xs font-mono"
                  value={googleClientSecret}
                  placeholder="GOCSPX-xxxxxxxx"
                  spellCheck={false}
                  autoComplete="off"
                  onChange={(event) => setGoogleClientSecret(event.currentTarget.value)}
                />
                <p className="text-[11px] text-muted-foreground">{t('cloudSync.oauth.googleSecretHint')}</p>
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </section>
  );
};

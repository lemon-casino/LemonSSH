import { useSyncExternalStore } from 'react';

import { netcattyBridge } from '../../infrastructure/services/netcattyBridge';
import {
  getGoogleClientSecretSnapshot,
  getOAuthClientIdsSnapshot,
  setOAuthClientId,
  setOAuthClientSecret,
  subscribeOAuthClientIds,
  type OAuthClientIds,
  type OAuthProvider,
} from '../../infrastructure/services/cloudSync/oauthClientIds';

export type { OAuthProvider } from '../../infrastructure/services/cloudSync/oauthClientIds';

/** Opens the provider's OAuth app console through the allow-listed bridge opener. */
function openProviderConsole(provider: OAuthProvider): Promise<void> {
  const opener = netcattyBridge.get()?.openProviderConsole;
  if (!opener) {
    console.error('Provider console bridge is unavailable');
    return Promise.resolve();
  }
  return opener(provider).catch((error: unknown) => {
    console.error(`Failed to open the ${provider} console page:`, error);
  });
}

/** React binding for the runtime OAuth client-ID store (Settings > Cloud Sync). */
export function useOAuthClientIds(): {
  ids: OAuthClientIds;
  setClientId: (provider: OAuthProvider, clientId: string) => void;
  googleClientSecret: string;
  setGoogleClientSecret: (secret: string) => void;
  openProviderConsole: (provider: OAuthProvider) => Promise<void>;
} {
  const ids = useSyncExternalStore(subscribeOAuthClientIds, getOAuthClientIdsSnapshot);
  const googleClientSecret = useSyncExternalStore(subscribeOAuthClientIds, getGoogleClientSecretSnapshot);
  return {
    ids,
    setClientId: setOAuthClientId,
    googleClientSecret,
    setGoogleClientSecret: (secret) => setOAuthClientSecret('google', secret),
    openProviderConsole,
  };
}

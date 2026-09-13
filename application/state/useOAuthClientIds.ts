import { useSyncExternalStore } from 'react';

import {
  getGoogleClientSecretSnapshot,
  getOAuthClientIdsSnapshot,
  setOAuthClientId,
  setOAuthClientSecret,
  subscribeOAuthClientIds,
  type OAuthClientIds,
  type OAuthProvider,
} from '../../infrastructure/services/cloudSync/oauthClientIds';

/** React binding for the runtime OAuth client-ID store (Settings > Cloud Sync). */
export function useOAuthClientIds(): {
  ids: OAuthClientIds;
  setClientId: (provider: OAuthProvider, clientId: string) => void;
  googleClientSecret: string;
  setGoogleClientSecret: (secret: string) => void;
} {
  const ids = useSyncExternalStore(subscribeOAuthClientIds, getOAuthClientIdsSnapshot);
  const googleClientSecret = useSyncExternalStore(subscribeOAuthClientIds, getGoogleClientSecretSnapshot);
  return {
    ids,
    setClientId: setOAuthClientId,
    googleClientSecret,
    setGoogleClientSecret: (secret) => setOAuthClientSecret('google', secret),
  };
}

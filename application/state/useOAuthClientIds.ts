import { useSyncExternalStore } from 'react';

import {
  getOAuthClientIds,
  setOAuthClientId,
  subscribeOAuthClientIds,
  type OAuthClientIds,
  type OAuthProvider,
} from '../../infrastructure/services/cloudSync/oauthClientIds';

/** React binding for the runtime OAuth client-ID store (Settings > Cloud Sync). */
export function useOAuthClientIds(): {
  ids: OAuthClientIds;
  setClientId: (provider: OAuthProvider, clientId: string) => void;
} {
  const ids = useSyncExternalStore(subscribeOAuthClientIds, getOAuthClientIds);
  return { ids, setClientId: setOAuthClientId };
}

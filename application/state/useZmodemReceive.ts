import { useCallback, useState } from 'react';
import { netcattyBridge } from '../../infrastructure/services/netcattyBridge';
import { useI18n } from '../i18n/I18nProvider';
import { toast } from '../../components/ui/toast';

type ReceiveBridge = Pick<NetcattyBridge, 'selectDirectory' | 'receiveZmodem'>;
export async function receiveZmodemIntoDirectory(bridge: ReceiveBridge, sessionId: string, title: string, notifyReady: () => void): Promise<void> {
  if (!bridge.selectDirectory || !bridge.receiveZmodem) throw new Error('ZMODEM receive unavailable');
  const directory = await bridge.selectDirectory(title);
  if (!directory) return;
  notifyReady();
  const result = await bridge.receiveZmodem(sessionId, directory);
  if (!result.success) throw new Error(result.error || 'ZMODEM receive failed');
}

export function useZmodemReceive(sessionId: string, connected: boolean) {
  const { t } = useI18n();
  const [busy, setBusy] = useState(false);
  const bridge = netcattyBridge.get();
  const receive = useCallback(() => {
    if (!bridge || busy) return;
    setBusy(true);
    void receiveZmodemIntoDirectory(bridge, sessionId, t('terminal.menu.receiveZmodem'), () => {
      toast.info(t('terminal.zmodem.receiveReady'));
    }).catch(error => {
      console.error('ZMODEM receive failed', error);
      toast.error(t('terminal.zmodem.receiveFailed'));
    }).finally(() => setBusy(false));
  }, [bridge, busy, sessionId, t]);
  return connected && !busy && bridge?.receiveZmodem && bridge.selectDirectory ? receive : undefined;
}

import { lemonsshBridge } from "./lemonsshBridge";

export const getCredentialProtectionAvailability = async (): Promise<boolean | null> => {
  const bridge = lemonsshBridge.get();
  if (!bridge?.credentialsAvailable) return null;

  try {
    return await bridge.credentialsAvailable();
  } catch {
    return null;
  }
};

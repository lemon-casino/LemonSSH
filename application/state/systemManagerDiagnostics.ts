import { lemonsshBridge } from '../../infrastructure/services/lemonsshBridge';

export async function writeSystemManagerDiagnostic(
  message: string,
  extra?: Record<string, unknown>,
) {
  try {
    await lemonsshBridge.get()?.logDiagnostic?.({
      source: 'system-manager',
      message,
      extra,
    });
  } catch {
    // Diagnostics must never block the user action being diagnosed.
  }
}

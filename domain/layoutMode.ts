export type LayoutMode = 'classic' | 'workbench';

export const DEFAULT_LAYOUT_MODE: LayoutMode = 'classic';

/** Returns null for anything that is not a recognized layout mode value. */
export function parseLayoutMode(raw: unknown): LayoutMode | null {
  if (typeof raw !== 'string') return null;
  if (raw === 'classic' || raw === 'workbench') return raw;
  return null;
}

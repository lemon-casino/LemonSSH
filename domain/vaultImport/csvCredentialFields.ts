const FORMULA_PREFIX = /^[=+\-@\t\r]/u;
const KEY_PATH_MARKER = "__lemonssh_csv_keypath_v1__:";
const PASSPHRASE_MARKER = "__lemonssh_csv_passphrase_v1__:";
// Pre-rename CSV exports carry the netcatty markers; imports keep decoding
// both so old export files stay readable.
const LEGACY_KEY_PATH_MARKER = "__netcatty_csv_keypath_v1__:";
const LEGACY_PASSPHRASE_MARKER = "__netcatty_csv_passphrase_v1__:";

const encodeMarkedField = (value: string, markers: [string, string]): string => (
  FORMULA_PREFIX.test(value) || markers.some((marker) => value.startsWith(marker))
    ? `${markers[0]}${encodeURIComponent(value)}`
    : value
);

const decodeMarkedField = (value: string, markers: [string, string]): string => {
  const marker = markers.find((candidate) => value.startsWith(candidate));
  if (!marker) return value;
  try {
    return decodeURIComponent(value.slice(marker.length));
  } catch {
    return value;
  }
};

export const encodeCsvKeyPath = (value: string): string => (
  encodeMarkedField(value, [KEY_PATH_MARKER, LEGACY_KEY_PATH_MARKER])
);

export const decodeCsvKeyPath = (value: string): string => (
  decodeMarkedField(value, [KEY_PATH_MARKER, LEGACY_KEY_PATH_MARKER])
);

export const encodeCsvPassphrase = (value: string): string => (
  encodeMarkedField(value, [PASSPHRASE_MARKER, LEGACY_PASSPHRASE_MARKER])
);

export const decodeCsvPassphrase = (value: string): string => (
  decodeMarkedField(value, [PASSPHRASE_MARKER, LEGACY_PASSPHRASE_MARKER])
);

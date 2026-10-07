import type { KeyType } from './models';

export type ImportedKeyRole = 'private' | 'public' | 'certificate' | 'unknown';

export interface ImportedKeyFile {
  name: string;
  content: string;
}

export interface ClassifiedKeyFile extends ImportedKeyFile {
  role: ImportedKeyRole;
  type: KeyType;
  stem: string;
}

export interface GroupedImportedKey {
  label: string;
  type: KeyType;
  privateKey?: string;
  publicKey?: string;
  certificate?: string;
}

const PRIVATE_BEGIN = /-----BEGIN ([A-Z0-9 ]+)-----/;
const PUBLIC_LINE = /^(ssh-(?:rsa|ed25519|dss)|ecdsa-sha2-nistp(?:256|384|521)|sk-(?:ssh-ed25519|ecdsa-sha2-nistp256)@openssh\.com)(?:-cert-v01@openssh\.com)?\s+\S+/m;
const CERT_LINE = /^(ssh-(?:rsa|ed25519|dss)|ecdsa-sha2-nistp(?:256|384|521)|sk-(?:ssh-ed25519|ecdsa-sha2-nistp256)@openssh\.com)-cert-v01@openssh\.com\s+\S+/m;

export function sshKeyFileStem(fileName: string): string {
  return fileName
    .replace(/\\/g, '/')
    .split('/')
    .pop()
    ?.replace(/\.(pub|pem|key|ppk|cert|crt)$/i, '')
    .replace(/-cert$/i, '')
    .replace(/[._-]?(pub|private|priv|public)$/i, '')
    || fileName;
}

export function detectImportedKeyType(content: string): KeyType {
  const text = content.toLowerCase();
  if (text.includes('rsa')) return 'RSA';
  if (text.includes('ecdsa') || text.includes('nistp') || /\bec\s/.test(text)) return 'ECDSA';
  return 'ED25519';
}

export function classifyImportedKeyContent(content: string): { role: ImportedKeyRole; type: KeyType } {
  const trimmed = content.trim();
  const type = detectImportedKeyType(trimmed);
  const begin = trimmed.match(PRIVATE_BEGIN);
  if (begin) {
    const kind = begin[1].toUpperCase();
    if (kind.includes('CERTIFICATE')) return { role: 'certificate', type };
    if (kind.includes('PUBLIC')) return { role: 'public', type };
    return { role: 'private', type };
  }
  if (CERT_LINE.test(trimmed)) return { role: 'certificate', type };
  if (PUBLIC_LINE.test(trimmed)) return { role: 'public', type };
  return { role: 'unknown', type };
}

export function classifyImportedKeyFile(file: ImportedKeyFile): ClassifiedKeyFile {
  const classified = classifyImportedKeyContent(file.content);
  let role = classified.role;
  if (role === 'unknown') {
    if (/\.pub$/i.test(file.name)) role = 'public';
    else if (/\.(cert|crt)$/i.test(file.name)) role = 'certificate';
    else if (/\.(pem|key|ppk)$/i.test(file.name) || !/\./.test(file.name.split(/[/\\]/).pop() ?? '')) {
      role = 'private';
    }
  }
  return {
    ...file,
    role,
    type: classified.type,
    stem: sshKeyFileStem(file.name),
  };
}

function mergeIntoGroup(group: GroupedImportedKey, file: ClassifiedKeyFile): void {
  if (file.role === 'private' && !group.privateKey) group.privateKey = file.content.trim();
  if (file.role === 'public' && !group.publicKey) group.publicKey = file.content.trim();
  if (file.role === 'certificate' && !group.certificate) group.certificate = file.content.trim();
  if (file.type) group.type = file.type;
}

export function groupImportedKeyFiles(files: readonly ImportedKeyFile[]): GroupedImportedKey[] {
  const classified = files
    .map((file) => classifyImportedKeyFile({ name: file.name, content: file.content.trim() }))
    .filter((file) => file.content.length > 0);
  const byStem = new Map<string, GroupedImportedKey>();
  const unmatched: ClassifiedKeyFile[] = [];

  for (const file of classified) {
    const existing = byStem.get(file.stem);
    if (existing) {
      mergeIntoGroup(existing, file);
      continue;
    }
    if (file.role === 'private' || file.role === 'public' || file.role === 'certificate') {
      const group: GroupedImportedKey = {
        label: file.stem,
        type: file.type,
      };
      mergeIntoGroup(group, file);
      byStem.set(file.stem, group);
      continue;
    }
    unmatched.push(file);
  }

  for (const file of unmatched) {
    const existing = byStem.get(file.stem);
    if (existing) {
      if (!existing.privateKey) existing.privateKey = file.content;
      continue;
    }
    byStem.set(file.stem, {
      label: file.stem,
      type: file.type,
      privateKey: file.content,
    });
  }

  const groups = [...byStem.values()];
  const privateOnly = groups.filter((group) => group.privateKey && !group.publicKey);
  const publicOnly = groups.filter((group) => group.publicKey && !group.privateKey);
  if (privateOnly.length === 1 && publicOnly.length === 1 && privateOnly[0] !== publicOnly[0]) {
    privateOnly[0].publicKey = publicOnly[0].publicKey;
    if (publicOnly[0].certificate && !privateOnly[0].certificate) {
      privateOnly[0].certificate = publicOnly[0].certificate;
    }
    return groups.filter((group) => group !== publicOnly[0]);
  }

  const certOnly = groups.filter((group) => group.certificate && !group.privateKey && !group.publicKey);
  const keyGroups = groups.filter((group) => group.privateKey || group.publicKey);
  if (certOnly.length === 1 && keyGroups.length === 1) {
    keyGroups[0].certificate = certOnly[0].certificate;
    return groups.filter((group) => group !== certOnly[0]);
  }

  return groups;
}

export function applyImportedKeyGroupToDraft(
  draft: { label?: string; type?: KeyType; privateKey?: string; publicKey?: string; certificate?: string },
  group: GroupedImportedKey,
): typeof draft {
  return {
    ...draft,
    label: draft.label || group.label,
    type: group.type || draft.type,
    privateKey: group.privateKey ?? draft.privateKey,
    publicKey: group.publicKey ?? draft.publicKey,
    certificate: group.certificate ?? draft.certificate,
  };
}

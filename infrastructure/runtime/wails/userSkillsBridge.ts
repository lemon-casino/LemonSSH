export interface NativeUserSkillsBindings {
  GetStatus: () => Promise<UserSkillsStatusResult>;
  OpenFolder: () => Promise<UserSkillsStatusResult>;
  BuildContext: (prompt: string, selectedSkillSlugs: string[]) => Promise<{ ok: boolean; context?: string; error?: string }>;
}

export function createUserSkillsBridge(bindings: NativeUserSkillsBindings | undefined): Partial<NetcattyBridge> {
  const required = () => {
    if (!bindings) throw new Error('Native user skills service is unavailable');
    return bindings;
  };
  return {
    aiUserSkillsGetStatus: () => required().GetStatus(),
    aiUserSkillsOpenFolder: () => required().OpenFolder(),
    aiUserSkillsBuildContext: (prompt, selectedSkillSlugs) => required().BuildContext(prompt, selectedSkillSlugs ?? []),
  };
}

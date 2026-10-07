export type CloseBehavior = "minimize" | "quit";

export function shouldPromptForCloseBehavior(chosen: CloseBehavior | null | undefined): boolean {
  return chosen !== "minimize" && chosen !== "quit";
}

export function resolveCloseAction(chosen: CloseBehavior | null | undefined): CloseBehavior | "prompt" {
  return shouldPromptForCloseBehavior(chosen) ? "prompt" : chosen;
}

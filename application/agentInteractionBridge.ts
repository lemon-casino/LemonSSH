import { lemonsshBridge } from "../infrastructure/services/lemonsshBridge";

export type AgentInteractionBridge = Pick<
  LemonSSHBridge,
  "onAgentInteraction" | "onAgentInteractionCleared" | "agentPendingInteractions" | "agentRespondInteraction"
>;

export function getAgentInteractionBridge(): AgentInteractionBridge | undefined {
  return lemonsshBridge.get();
}

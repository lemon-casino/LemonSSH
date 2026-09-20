import { netcattyBridge } from "../infrastructure/services/netcattyBridge";

export type AgentInteractionBridge = Pick<
  NetcattyBridge,
  "onAgentInteraction" | "onAgentInteractionCleared" | "agentPendingInteractions" | "agentRespondInteraction"
>;

export function getAgentInteractionBridge(): AgentInteractionBridge | undefined {
  return netcattyBridge.get();
}

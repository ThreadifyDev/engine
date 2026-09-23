import type { AgentMessage } from './agent-preview';

type ToolActivity = NonNullable<AgentMessage['tools']>[number];

export function updateToolActivity(
  tools: ToolActivity[] = [],
  activity: ToolActivity,
  claimStreamedCall = false,
): ToolActivity[] {
  let index = tools.findIndex(tool => tool.id === activity.id);
  // Harnest's client request ID differs from its streamed model tool ID.
  // Adopt that pending entry once; subsequent continuations keep unique IDs.
  if (index < 0 && claimStreamedCall) {
    index = tools.findIndex(tool => tool.status === 'running' && tool.name === activity.name);
  }
  return index < 0 ? [...tools, activity] : tools.map((tool, at) => at === index ? activity : tool);
}

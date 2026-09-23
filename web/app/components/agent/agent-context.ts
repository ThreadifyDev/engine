import { createContext, useContext } from 'react';
import type { AgentContext, AgentMessage } from './agent-preview';
import type { ProfileViewDesignerBridge } from '../profiles/view/profile-view';
import type { ContractDraft } from './client-tools';
import type { AgentStatus } from './agent-status';

type AgentState = {
  isEnabled: boolean;
  agentStatus: AgentStatus;
  checkingAgentStatus: boolean;
  refreshAgentStatus: () => void;
  registerProfileDesigner: (designer: ProfileViewDesignerBridge) => () => void;
  applyProfileProposal: (message: AgentMessage) => void;
  isOpen: boolean;
  isSupported: boolean;
  isCompact: boolean;
  openAgent: () => void;
  closeAgent: () => void;
  context: AgentContext;
  includeContext: boolean;
  setIncludeContext: (include: boolean) => void;
  messages: AgentMessage[];
  composer: string;
  setComposer: (value: string) => void;
  sendMessage: (text: string) => void;
  newConversation: () => void;
  isSending: boolean;
  error: string;
  stop: () => void;
  contractDraft: ContractDraft;
  editContractDraft: (source: string) => void;
  setContractEditorOpen: (open: boolean) => void;
};

export const AgentStateContext = createContext<AgentState | null>(null);

export function useAgent() {
  const state = useContext(AgentStateContext);
  if (!state) throw new Error('useAgent must be used inside AgentProvider');
  return state;
}

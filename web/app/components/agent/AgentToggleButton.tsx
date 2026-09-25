import { Sparkles } from 'lucide-react';
import { useAgent } from './agent-context';

export default function AgentToggleButton() {
  const { isEnabled, agentStatus, isOpen, isSupported, openAgent } = useAgent();
  if (!isEnabled || !isSupported || isOpen) return null;
  return (
    <button data-agent-launcher onClick={openAgent}
      aria-label="Ask agent"
      aria-controls="threadify-agent" aria-expanded={isOpen}
      title={agentStatus.status === 'ready' ? 'Ask agent (⌘/Ctrl+J)' : agentStatus.status === 'starting' ? 'Agent is starting' : 'Agent is unavailable — check connection'}
      className="fixed bottom-[max(1rem,env(safe-area-inset-bottom))] right-4 z-30 flex h-12 w-12 items-center justify-center rounded-full bg-[#172e28] text-white shadow-lg transition-colors hover:bg-[#25463d] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-700 sm:right-6">
      <Sparkles className="h-5 w-5" strokeWidth={1.6} />
    </button>
  );
}

import { Sparkles } from 'lucide-react';
import { useAgent } from './agent-context';

export default function AgentToggleButton({ dark = false }: { dark?: boolean }) {
  const { isEnabled, agentStatus, isOpen, isSupported, openAgent } = useAgent();
  if (!isEnabled || !isSupported || isOpen) return null;
  return (
    <button data-agent-launcher onClick={openAgent}
      aria-label="Ask agent"
      aria-controls="threadify-agent" aria-expanded={isOpen}
      title={agentStatus.status === 'ready' ? 'Ask agent (⌘/Ctrl+J)' : agentStatus.status === 'starting' ? 'Agent is starting' : 'Agent is unavailable — check connection'}
      className={`flex h-9 w-9 items-center justify-center rounded-lg transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-700 ${dark ? 'text-stone-300 hover:bg-stone-800 hover:text-white' : 'text-stone-400 hover:bg-stone-50 hover:text-stone-600'}`}>
      <Sparkles className="h-[18px] w-[18px]" strokeWidth={1.4} />
    </button>
  );
}

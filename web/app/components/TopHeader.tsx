import AgentToggleButton from './agent/AgentToggleButton';

export default function TopHeader() {
  return (
    <header className="flex h-14 items-center justify-end border-b border-stone-100 bg-white px-6 lg:px-8">
      <AgentToggleButton />
    </header>
  );
}

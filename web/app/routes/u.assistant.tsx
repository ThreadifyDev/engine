import type { MetaFunction } from '@remix-run/node';
import ThreadChat from '~/components/ThreadChat';
import AppLayout from '~/components/AppLayout';

export const meta: MetaFunction = () => {
  return [
    { title: 'AI Assistant - Threadify' },
    { name: 'description', content: 'AI-powered assistant for contract generation and thread analysis' },
  ];
};

export default function AssistantPage() {
  return (
    <AppLayout>
      <div className="flex flex-col bg-gray-50" style={{ height: 'calc(100vh - 0px)' }}>
        {/* Header */}
        <div className="bg-white border-b border-gray-200 px-8 py-8 flex-shrink-0">
          <h1 className="text-2xl font-bold text-gray-900 mb-2">Threadify AI</h1>
          <p className="text-gray-600">
            Generate contracts, analyze threads, and get insights from your execution data
          </p>
        </div>

        {/* AI Chat Interface */}
        <div className="flex-1 overflow-hidden px-8 py-6">
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm h-full overflow-hidden">
            <ThreadChat />
          </div>
        </div>
      </div>
    </AppLayout>
  );
}

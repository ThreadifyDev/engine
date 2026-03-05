import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Loader2, ChevronRight } from 'lucide-react';
import { graphqlClient, type StepStateInfo } from '~/lib/graphql';

// Helper function to calculate execution time from startedAt and finishedAt
function calculateExecutionTime(startedAt?: string, finishedAt?: string): string | null {
  if (!startedAt || !finishedAt) return null;
  
  const start = new Date(startedAt).getTime();
  const end = new Date(finishedAt).getTime();
  const diff = end - start;
  
  if (diff < 1000) return `${diff}ms`;
  if (diff < 60000) return `${(diff / 1000).toFixed(2)}s`;
  return `${(diff / 60000).toFixed(2)}m`;
}

export function StepHistoryContent({
  step,
  threadId,
}: {
  step: StepStateInfo;
  threadId: string;
}) {
  const [expandedIndex, setExpandedIndex] = useState<number | null>(0);

  // Fetch step history on-demand
  const { data: history, isLoading } = useQuery({
    queryKey: ['stepHistory', threadId, step.stepName, step.idempotencyKey],
    queryFn: () => graphqlClient.getStepHistory(threadId, step.stepName, step.idempotencyKey, 100),
  });

  // Extract unique actor IDs from history and resolve them
  const actorIds = Array.from(new Set(history?.map(h => h.actor).filter(Boolean) || []));
  const { data: resolvedActors } = useQuery({
    queryKey: ['resolveActorsHistory', actorIds],
    queryFn: () => graphqlClient.resolveActors(actorIds as string[]),
    enabled: actorIds.length > 0,
  });

  // Create actor ID to name map
  const actorMap = new Map<string, string>();
  resolvedActors?.forEach(actor => {
    actorMap.set(actor.id, actor.name);
  });

  return (
    <div>
          {isLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="w-6 h-6 animate-spin text-gray-400" />
            </div>
          ) : history && history.length > 0 ? (
            <div className="space-y-2">
              {history.map((item, idx) => {
                const isExpanded = expandedIndex === idx;

                return (
                  <div
                    key={idx}
                    className="border border-gray-200 rounded-lg overflow-hidden"
                  >
                    {/* Accordion Header */}
                    <button
                      onClick={() => setExpandedIndex(isExpanded ? null : idx)}
                      className="w-full px-3 py-2.5 text-left hover:bg-gray-50 transition-colors flex items-center justify-between"
                    >
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium text-gray-900">
                            Attempt {item.attempt}
                          </span>
                          <span className={`text-xs px-1.5 py-0.5 rounded-full ${
                            item.status === 'success' ? 'bg-green-100 text-green-700' :
                            item.status === 'failed' ? 'bg-red-100 text-red-700' :
                            'bg-gray-100 text-gray-700'
                          }`}>
                            {item.status}
                          </span>
                          {(() => {
                            const executionTime = calculateExecutionTime(item.startedAt, item.finishedAt);
                            return executionTime ? (
                              <span className="text-xs px-1.5 py-0.5 rounded-full bg-blue-100 text-blue-700">
                                {executionTime}
                              </span>
                            ) : null;
                          })()}
                        </div>
                        <div className="text-xs text-gray-500 mt-0.5">
                          {new Date(item.timestamp).toLocaleString()}
                        </div>
                      </div>
                      <ChevronRight className={`w-4 h-4 text-gray-400 transition-transform flex-shrink-0 ${
                        isExpanded ? 'rotate-90' : ''
                      }`} />
                    </button>

                    {/* Accordion Content */}
                    {isExpanded && (
                      <div className="px-3 pb-3 space-y-3 border-t border-gray-100">
                        {/* Actor & Service */}
                        <div className="pt-3 space-y-2">
                          {item.actor && (
                            <div>
                              <div className="text-xs font-medium text-gray-500 mb-1">Actor</div>
                              <div className="text-sm text-gray-900">{actorMap.get(item.actor) || item.actor}</div>
                            </div>
                          )}
                          {item.actorService && (
                            <div>
                              <div className="text-xs font-medium text-gray-500 mb-1">Service</div>
                              <div className="text-sm text-gray-900 font-mono">{item.actorService}</div>
                            </div>
                          )}
                        </div>

                        {/* Context */}
                        {item.context && (
                          <div>
                            <div className="text-xs font-medium text-gray-500 mb-1">Context</div>
                            <pre className="text-xs bg-gray-50 p-2 rounded border border-gray-200 overflow-x-auto font-mono">
                              {(() => {
                                try {
                                  const parsed = JSON.parse(item.context);
                                  const contextObj = typeof parsed === 'string' ? JSON.parse(parsed) : parsed;
                                  // Sort keys for consistent display
                                  const sortedObj = Object.keys(contextObj)
                                    .sort()
                                    .reduce((acc, key) => {
                                      acc[key] = contextObj[key];
                                      return acc;
                                    }, {} as Record<string, any>);
                                  return JSON.stringify(sortedObj, null, 2);
                                } catch (e) {
                                  return item.context;
                                }
                              })()}
                            </pre>
                          </div>
                        )}

                        {/* Error/Success Message */}
                        {item.metadata && (
                          <div>
                            <div className={`text-xs font-medium mb-1 ${
                              item.status === 'failed' ? 'text-red-600' : 
                              item.status === 'success' ? 'text-green-600' : 
                              'text-gray-600'
                            }`}>
                              {item.status === 'failed' ? 'Error Message' : 
                               item.status === 'success' ? 'Success Message' : 'Message'}
                            </div>
                            <div className={`text-sm ${
                              item.status === 'failed' ? 'text-red-700' : 
                              item.status === 'success' ? 'text-green-700' : 
                              'text-gray-700'
                            }`}>
                              {(() => {
                                try {
                                  const parsed = JSON.parse(item.metadata);
                                  if (typeof parsed === 'object' && parsed !== null) {
                                    if (parsed.message) {
                                      return (
                                        <div className="space-y-2">
                                          <p>{parsed.message}</p>
                                          {Object.keys(parsed).length > 1 && (
                                            <pre className="text-xs bg-gray-50 p-2 rounded border border-gray-200 overflow-x-auto font-mono mt-2">
                                              {JSON.stringify(parsed, null, 2)}
                                            </pre>
                                          )}
                                        </div>
                                      );
                                    } else {
                                      return (
                                        <pre className="text-xs bg-gray-50 p-2 rounded border border-gray-200 overflow-x-auto font-mono">
                                          {JSON.stringify(parsed, null, 2)}
                                        </pre>
                                      );
                                    }
                                  }
                                  return String(parsed);
                                } catch {
                                  return item.metadata;
                                }
                              })()}
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          ) : (
            <div className="text-sm text-gray-500 text-center py-8">
              No history available
            </div>
          )}
    </div>
  );
}

import { useState } from 'react';
import { ChevronDown, ChevronRight, Copy, Check } from 'lucide-react';
import { type SubStep } from '~/lib/graphql';
import { formatDistanceToNow } from 'date-fns';
import RightSidebar from '~/components/RightSidebar';

// Format a timestamp string in a human-readable format with milliseconds
function formatTimestampMs(iso: string): string {
  const d = new Date(iso);
  const ms = d.getMilliseconds().toString().padStart(3, '0');
  const baseFormat = d.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
    hour12: true
  });
  return baseFormat.replace(/(\d{2})\s+(AM|PM)/, `$1.${ms} $2`);
}

interface SubStateSidebarProps {
  subSteps: SubStep[];
  stepName: string;
  stepStartedAt?: string;
  onClose: () => void;
  onBack: () => void;
}

export function SubStateSidebar({ subSteps, stepName, stepStartedAt, onClose, onBack }: SubStateSidebarProps) {
  // Calculate time relative to step start
  const getRelativeTime = (recordedAt: string): string => {
    if (!stepStartedAt) return formatDistanceToNow(new Date(recordedAt), { addSuffix: true });
    
    const stepStart = new Date(stepStartedAt).getTime();
    const subStepRecorded = new Date(recordedAt).getTime();
    const diffMs = subStepRecorded - stepStart;
    
    // Debug logging
    console.log('Step Start:', stepStartedAt, '→', stepStart);
    console.log('SubStep Recorded:', recordedAt, '→', subStepRecorded);
    console.log('Diff (ms):', diffMs);
    
    if (diffMs < 0) return '0ms after step start';
    if (diffMs < 1000) return `${diffMs}ms after step start`;
    if (diffMs < 60000) return `${(diffMs / 1000).toFixed(2)}s after step start`;
    if (diffMs < 3600000) return `${(diffMs / 60000).toFixed(2)}m after step start`;
    return `${(diffMs / 3600000).toFixed(2)}h after step start`;
  };
  
  // Truncate step name if too long
  const truncatedStepName = stepName.length > 20 ? stepName.substring(0, 20) + '...' : stepName;
  const [expandedIndex, setExpandedIndex] = useState<number>(0);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  const copyToClipboard = async (text: string, field: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedField(field);
      setTimeout(() => setCopiedField(null), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <RightSidebar
      isOpen={true}
      onClose={onClose}
      title={
        <div className="flex items-center gap-2">
          <button
            onClick={onBack}
            className="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 transition-colors"
          >
            <ChevronRight className="w-4 h-4 rotate-180" />
            Back
          </button>
          <span className="text-gray-300">|</span>
          <span>Sub State ({truncatedStepName})</span>
        </div>
      }
      width="lg"
    >
      <div>
          {subSteps.length === 0 ? (
            <div className="text-sm text-gray-500">No substates found</div>
          ) : (
            <div className="space-y-2">
              {subSteps.map((subStep, index) => (
                <div key={subStep.id} className="border border-gray-200 rounded-lg overflow-hidden">
                  {/* Accordion Header */}
                  <button
                    onClick={() => setExpandedIndex(expandedIndex === index ? -1 : index)}
                    className="w-full p-3 text-left hover:bg-gray-50 transition-colors"
                  >
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2 flex-1 min-w-0">
                        {expandedIndex === index ? (
                          <ChevronDown className="w-4 h-4 text-gray-400 flex-shrink-0" />
                        ) : (
                          <ChevronRight className="w-4 h-4 text-gray-400 flex-shrink-0" />
                        )}
                        <div className="flex-1 min-w-0">
                          <div className="font-medium text-sm text-gray-900 truncate">
                            {subStep.name}
                          </div>
                          <div className="text-xs text-gray-500 mt-0.5">
                            {getRelativeTime(subStep.recordedAt)}
                          </div>
                        </div>
                      </div>
                      <span className={`ml-2 px-2 py-0.5 text-xs font-medium rounded-full flex-shrink-0 ${
                        subStep.status === 'success' ? 'bg-green-100 text-green-800' :
                        subStep.status === 'failed' ? 'bg-red-100 text-red-800' :
                        subStep.status === 'in_progress' ? 'bg-blue-100 text-blue-800' :
                        'bg-gray-100 text-gray-800'
                      }`}>
                        {subStep.status}
                      </span>
                    </div>
                  </button>

                  {/* Accordion Content */}
                  {expandedIndex === index && (
                    <div className="border-t border-gray-200 p-3 bg-gray-50 space-y-3">
                      {/* Payload */}
                      {subStep.payload && Object.keys(subStep.payload).length > 0 && (
                        <div className="space-y-1">
                          <div className="text-xs font-medium text-gray-600">Payload</div>
                          <div className="relative">
                            <pre className="text-xs bg-white p-2 pr-8 rounded border border-gray-200 overflow-x-auto font-mono max-h-60">
                              {subStep.payload}
                            </pre>
                            <button
                              onClick={() => copyToClipboard(subStep.payload, `payload-${index}`)}
                              className="absolute top-2 right-2 p-1 text-gray-400 hover:text-gray-600 transition-colors"
                              title="Copy to clipboard"
                            >
                              {copiedField === `payload-${index}` ? (
                                <Check className="w-3 h-3 text-green-600" />
                              ) : (
                                <Copy className="w-3 h-3" />
                              )}
                            </button>
                          </div>
                        </div>
                      )}

                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
      </div>
    </RightSidebar>
  );
}

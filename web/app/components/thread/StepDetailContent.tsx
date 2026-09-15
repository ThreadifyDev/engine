import { useState } from 'react';
import { formatDistanceToNow } from 'date-fns';
import { useQuery } from '@tanstack/react-query';
import {
  CheckCircle2,
  XCircle,
  Clock,
  RefreshCw,
  Calendar,
  Hash,
  Code,
  ChevronRight,
  Copy,
  Check,
  ShieldAlert,
  AlertTriangle,
  Info,
  User,
  Layers,
} from 'lucide-react';
import { graphqlClient, type StepStateInfo, type ValidationResultInfo } from '~/lib/graphql';

// Calculate duration between two timestamps
function calculateDuration(startTime?: string, endTime?: string): string | null {
  if (!startTime || !endTime) return null;
  const start = new Date(startTime).getTime();
  const end = new Date(endTime).getTime();
  const durationMs = end - start;
  
  if (durationMs < 0) return null;
  if (durationMs === 0) return '< 1ms';
  if (durationMs < 1000) return `${durationMs}ms`;
  if (durationMs < 60000) return `${(durationMs / 1000).toFixed(2)}s`;
  if (durationMs < 3600000) return `${(durationMs / 60000).toFixed(2)}m`;
  return `${(durationMs / 3600000).toFixed(2)}h`;
}

// Format a timestamp string in a human-readable format with milliseconds.
// Accepts ISO 8601 with or without sub-second component.
function formatTimestampMs(iso: string): string {
  const d = new Date(iso);
  const ms = d.getMilliseconds().toString().padStart(3, '0');
  // Format: "Feb 20, 2026 at 5:48:09.732 PM"
  const baseFormat = d.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
    hour12: true
  });
  // Insert milliseconds before AM/PM
  return baseFormat.replace(/(\d{2})\s+(AM|PM)/, `$1.${ms} $2`);
}

// Actor Section Component - Resolves actor ID to name
function ActorSection({ actorId, actorService }: { actorId: string; actorService?: string }) {
  const { data: actors, isLoading } = useQuery({
    queryKey: ['resolveActor', actorId],
    queryFn: () => graphqlClient.resolveActors([actorId]),
    enabled: !!actorId,
  });

  const actor = actors?.[0];

  return (
    <div className="space-y-2">
      <h5 className="font-bold text-gray-900 flex items-center gap-2">
        <User className="w-4 h-4" />
        Actor
      </h5>
      <div className="bg-gray-50 border-2 border-gray-200 p-3 rounded">
        {isLoading ? (
          <div className="text-sm text-gray-500">Loading...</div>
        ) : actor ? (
          <>
            <div className="font-medium text-black">{actor.name}</div>
            {actor.company && (
              <div className="text-xs text-gray-600 mt-1">{actor.company}</div>
            )}
            {actorService && (
              <div className="mt-2 flex items-center gap-2">
                <span className="text-xs text-gray-500">Service:</span>
                <span className="text-xs font-mono text-blue-600">{actorService}</span>
              </div>
            )}
          </>
        ) : (
          <div className="text-sm text-gray-500">Actor not found</div>
        )}
      </div>
    </div>
  );
}

export function StepDetailContent({ 
  step, 
  threadId, 
  showContext, 
  onToggleContext,
  validations,
  onShowHistory,
  onShowViolations,
  onShowSubState,
}: { 
  step: StepStateInfo; 
  threadId: string; 
  showContext: boolean; 
  onToggleContext: () => void;
  validations: ValidationResultInfo[];
  onShowHistory: (step: StepStateInfo) => void;
  onShowViolations?: (step: StepStateInfo) => void;
  onShowSubState?: (subSteps: any[], stepName: string, stepStartedAt?: string) => void;
}) {
  const [showValidations, setShowValidations] = useState(false);
  const [validationFilter, setValidationFilter] = useState<'all' | 'critical' | 'warning' | 'info'>('all');
  const [copiedField, setCopiedField] = useState<string | null>(null);
  const [showErrorMessage, setShowErrorMessage] = useState(true);

  const copyToClipboard = async (text: string, field: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopiedField(field);
      setTimeout(() => setCopiedField(null), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  // Count validations for this step
  const stepValidations = validations.filter(
    v => v.stepName === step.stepName && v.idempotencyKey === step.idempotencyKey
  );
  
  // Only show validations if there are actual failures (not just success)
  const hasActualIssues = stepValidations.some(v => 
    v.hasCriticalViolation || 
    v.criticalCount > 0 || 
    v.warningCount > 0 || 
    v.validations.length > 0
  );
  const hasCritical = stepValidations.some(v => v.hasCriticalViolation);

  // Determine severity for filtering
  const getSeverity = (validation: ValidationResultInfo): string => {
    if (validation.hasCriticalViolation || validation.criticalCount > 0) return 'critical';
    if (validation.warningCount > 0) return 'warning';
    return 'info';
  };

  // Filter validations based on selected filter
  const filteredValidations = stepValidations.filter(v => {
    if (validationFilter === 'all') return true;
    return getSeverity(v) === validationFilter;
  });

  return (
    <div className="space-y-6">
      {/* Step Name & Status */}
      <div>
        <div className="flex items-center gap-2 mb-1">
          <h4 className="text-2xl font-bold text-black">{step.stepName}</h4>
          <button
            onClick={() => copyToClipboard(`${step.stepName}:${step.idempotencyKey}`, 'header')}
            className="p-1.5 rounded hover:bg-gray-100 transition-colors"
            title="Copy step identifier"
          >
            {copiedField === 'header' ? (
              <Check className="w-4 h-4 text-green-600" />
            ) : (
              <Copy className="w-4 h-4 text-gray-500" />
            )}
          </button>
          {step.status === 'success' && <CheckCircle2 className="w-6 h-6 text-green-600" />}
          {step.status === 'failed' && <XCircle className="w-6 h-6 text-red-600" />}
          {step.status === 'in_progress' && <Clock className="w-6 h-6 text-blue-600" />}
          {step.status === 'pending' && <Clock className="w-6 h-6 text-gray-400" />}
        </div>
        
        <div className="flex items-center gap-2">
          <span className={`inline-flex px-2 py-0.5 text-xs font-medium rounded-full ${
            step.status === 'success' ? 'bg-green-100 text-green-800' :
            step.status === 'failed' ? 'bg-red-100 text-red-800' :
            step.status === 'violated' ? 'bg-orange-100 text-orange-800' :
            step.status === 'in_progress' ? 'bg-blue-100 text-blue-800' :
            'bg-gray-100 text-gray-800'
          }`}>
            {step.status}
          </span>

          <span className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full bg-gray-100 text-gray-800`}>
            <Clock className="w-3 h-3" />
            {calculateDuration(step.startedAt, step.finishedAt)}
          </span>
          
          {/* Retry Count */}
          {step.retryCount > 0 && (
            <div className="flex items-center gap-1 text-orange-700">
              <RefreshCw className="w-3.5 h-3.5" />
              <span className="text-xs font-medium">Retried {step.retryCount} time{step.retryCount > 1 ? 's' : ''}</span>
            </div>
          )}
        </div>
      </div>

      {/* Error/Success Message Display */}
      {step.history && step.history.length > 0 && step.history[0].metadata && (
        <div className={`rounded-lg border ${
          step.status === 'failed' ? 'bg-red-50/50 border-red-200' : 
          step.status === 'success' ? 'bg-green-50/50 border-green-200' : 
          'bg-yellow-50/50 border-yellow-200'
        }`}>
          <button
            onClick={() => setShowErrorMessage(!showErrorMessage)}
            className="w-full p-4 text-left transition-colors hover:bg-black/5"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                {step.status === 'failed' ? (
                  <XCircle className="w-4 h-4 text-red-600" />
                ) : step.status === 'success' ? (
                  <CheckCircle2 className="w-4 h-4 text-green-600" />
                ) : (
                  <Clock className="w-4 h-4 text-yellow-600" />
                )}
                <h5 className={`text-sm font-semibold ${
                  step.status === 'failed' ? 'text-red-900' : 
                  step.status === 'success' ? 'text-green-900' : 
                  'text-yellow-900'
                }`}>
                  {step.status === 'failed' ? 'Error Message' : 
                   step.status === 'success' ? 'Success Message' : 'Message'}
                </h5>
              </div>
              <ChevronRight className={`w-4 h-4 text-gray-400 transition-transform ${
                showErrorMessage ? 'rotate-90' : ''
              }`} />
            </div>
          </button>
          
          {showErrorMessage && (
            <div className="px-4 pb-4">
              <div className={`text-sm leading-relaxed ${
                step.status === 'failed' ? 'text-red-800' : 
                step.status === 'success' ? 'text-green-800' : 
                'text-yellow-800'
              }`}>
                {(() => {
                  const messageData = step.history[0].metadata;
                  if (!messageData) return null;
                  try {
                    const parsed = JSON.parse(messageData);
                    if (typeof parsed === 'object' && parsed !== null) {
                      if (parsed.message) {
                        return (
                          <div className="space-y-2">
                            <p>{parsed.message}</p>
                            {Object.keys(parsed).length > 1 && (
                              <pre className="text-xs bg-white/50 p-2 rounded border border-current/20 overflow-x-auto font-mono mt-2">
                                {JSON.stringify(parsed, null, 2)}
                              </pre>
                            )}
                          </div>
                        );
                      } else {
                        return (
                          <pre className="text-xs bg-white/50 p-2 rounded border border-current/20 overflow-x-auto font-mono">
                            {JSON.stringify(parsed, null, 2)}
                          </pre>
                        );
                      }
                    } else {
                      return <p>{String(parsed)}</p>;
                    }
                  } catch {
                    return <p>{messageData}</p>;
                  }
                })()}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Timestamps */}
      <div className="space-y-3">
        <h5 className="font-bold text-gray-900 flex items-center gap-2">
          <Calendar className="w-4 h-4" />
          Timeline
        </h5>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-600">First Seen:</span>
            <div className="text-right">
              <div className="font-medium">{formatDistanceToNow(new Date(step.firstSeenAt), { addSuffix: true })}</div>
              <div className="text-xs text-gray-500">{formatTimestampMs(step.firstSeenAt)}</div>
            </div>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-600">Last Updated:</span>
            <div className="text-right">
              <div className="font-medium">{formatDistanceToNow(new Date(step.lastUpdatedAt), { addSuffix: true })}</div>
              <div className="text-xs text-gray-500">{formatTimestampMs(step.lastUpdatedAt)}</div>
            </div>
          </div>
        </div>
      </div>

      {/* Actor */}
      {step.actor && (
        <ActorSection actorId={step.actor} actorService={step.actorService} />
      )}

      {/* IDs */}
      <div className="space-y-3">
        <h5 className="font-bold text-gray-900 flex items-center gap-2">
          <Hash className="w-4 h-4" />
          Identifiers
        </h5>
        <div className="space-y-2 text-sm">
          <div>
            <div className="text-gray-600 mb-1">Step ID:</div>
            <div className="relative group">
              <div className="font-mono text-xs bg-gray-100 p-2 pr-10 rounded break-all">
                {step.latestStepID}
              </div>
              <button
                onClick={() => copyToClipboard(step.latestStepID, 'stepId')}
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1.5 rounded hover:bg-gray-200 transition-colors opacity-0 group-hover:opacity-100"
                title="Copy Step ID"
              >
                {copiedField === 'stepId' ? (
                  <Check className="w-3.5 h-3.5 text-green-600" />
                ) : (
                  <Copy className="w-3.5 h-3.5 text-gray-600" />
                )}
              </button>
            </div>
          </div>
          <div>
            <div className="text-gray-600 mb-1">Idempotency Key:</div>
            <div className="relative group">
              <div className="font-mono text-xs bg-gray-100 p-2 pr-10 rounded break-all">
                {step.idempotencyKey}
              </div>
              <button
                onClick={() => copyToClipboard(step.idempotencyKey, 'idempKey')}
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1.5 rounded hover:bg-gray-200 transition-colors opacity-0 group-hover:opacity-100"
                title="Copy Idempotency Key"
              >
                {copiedField === 'idempKey' ? (
                  <Check className="w-3.5 h-3.5 text-green-600" />
                ) : (
                  <Copy className="w-3.5 h-3.5 text-gray-600" />
                )}
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* Previous Step */}
      {step.previousStep && (
        <div className="space-y-2">
          <h5 className="font-bold text-gray-900 flex items-center gap-2">
            <ChevronRight className="w-4 h-4 rotate-180" />
            Previous Step
          </h5>
          <div className="bg-gray-50 border-2 border-gray-200 p-3 rounded">
            <div className="font-medium">{step.previousStep}</div>
          </div>
        </div>
      )}

      {/* Action Buttons Section */}
      <div className="border-t border-gray-200 pt-6 space-y-0">
        {/* Show Context Data */}
        <button
          onClick={onToggleContext}
          className="w-full py-3 text-left transition-colors group hover:bg-gray-50"
        >
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Code className="w-4 h-4 text-gray-600" />
              <span className="text-sm font-medium text-grey-900">
                {showContext ? 'Hide' : 'Show'} Context Data
              </span>
            </div>
            <ChevronRight className={`w-4 h-4 text-gray-400 transition-transform ${
              showContext ? 'rotate-90' : ''
            }`} />
          </div>
        </button>

        {showContext && step.latestContext && (
          <div className="py-3 space-y-3">
            <div className="border border-gray-200 rounded-md p-3">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-gray-600">Latest Context</span>
              </div>
              <div className="relative">
                <pre className="text-xs bg-gray-50 p-2 rounded border border-gray-200 overflow-x-auto font-mono pr-8">
                  {(() => {
                    try {
                      const parsed = JSON.parse(step.latestContext);
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
                      return step.latestContext;
                    }
                  })()}
                </pre>
                <button
                  onClick={() => copyToClipboard(step.latestContext || '', 'context')}
                  className="absolute top-2 right-2 p-1 text-gray-400 hover:text-gray-600 transition-colors"
                  title="Copy to clipboard"
                >
                  {copiedField === 'context' ? (
                    <Check className="w-4 h-4 text-green-600" />
                  ) : (
                    <Copy className="w-4 h-4" />
                  )}
                </button>
              </div>
            </div>
          </div>
        )}

        {showContext && !step.latestContext && (
          <div className="py-3 text-sm text-gray-500 italic">
            No context data available
          </div>
        )}

        {/* View Step History */}
        <div className="border-t border-gray-200">
          <button
            onClick={() => onShowHistory(step)}
            className="w-full py-3 text-left transition-colors group hover:bg-gray-50"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Clock className="w-4 h-4 text-gray-600" />
                <span className="text-sm font-medium text-grey-900">
                  View Step History
                </span>
              </div>
              <ChevronRight className="w-4 h-4 text-gray-400" />
            </div>
          </button>
        </div>

        {/* View Sub State */}
        {step.subSteps && step.subSteps.length > 0 && onShowSubState && (
          <div className="border-t border-gray-200">
            <button
              onClick={() => onShowSubState(step.subSteps!, step.stepName, step.startedAt)}
              className="w-full py-3 text-left transition-colors group hover:bg-gray-50"
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Layers className="w-4 h-4 text-gray-600" />
                  <span className="text-sm font-medium text-grey-900">
                    View Sub State
                    {step.subSteps.length > 1 && (
                      <span className="ml-1.5 text-xs text-gray-400">({step.subSteps.length})</span>
                    )}
                  </span>
                </div>
                <ChevronRight className="w-4 h-4 text-gray-400" />
              </div>
            </button>
          </div>
        )}

        {/* View Violation History */}
        {step.status === 'violated' && onShowViolations && (
          <div className="border-t border-gray-200">
            <button
              onClick={() => onShowViolations(step)}
              className="w-full py-3 text-left transition-colors group hover:bg-gray-50"
            >
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <ShieldAlert className="w-4 h-4 text-orange-600" />
                  <span className="text-sm font-medium text-grey-900">
                    View Violation History
                  </span>
                </div>
                <ChevronRight className="w-4 h-4 text-gray-400" />
              </div>
            </button>
          </div>
        )}
      </div>

      {/* View Validations - Only show if there are actual issues */}
      {hasActualIssues && (
        <div className="border-t border-gray-200 pt-6">
          <button
            onClick={() => setShowValidations(!showValidations)}
            className="w-full px-3 py-3 text-left transition-colors group hover:bg-gray-50"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                {hasCritical ? (
                  <AlertTriangle className="w-4 h-4 text-red-600" />
                ) : (
                  <AlertTriangle className="w-4 h-4 text-yellow-600" />
                )}
                <span className="text-sm font-medium text-gray-900">
                  Validation Issues ({stepValidations.filter(v => v.validations.length > 0 || v.criticalCount > 0 || v.warningCount > 0).length})
                </span>
              </div>
              <ChevronRight className={`w-4 h-4 text-gray-400 transition-transform ${
                showValidations ? 'rotate-90' : ''
              }`} />
            </div>
          </button>

          {showValidations && (
            <div className="mt-3 space-y-3">
              {/* Filter Dropdown - minimal style */}
              <div className="flex items-center gap-2 pb-2 border-b border-gray-200">
                <label className="text-xs font-medium text-gray-500 uppercase tracking-wide">Filter</label>
                <select
                  value={validationFilter}
                  onChange={(e) => setValidationFilter(e.target.value as any)}
                  className="text-sm border border-gray-200 rounded-md px-2 py-1 bg-white text-gray-700 focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500"
                >
                  <option value="all">All ({stepValidations.length})</option>
                  <option value="critical">Critical ({stepValidations.filter(v => getSeverity(v) === 'critical').length})</option>
                  <option value="warning">Warning ({stepValidations.filter(v => getSeverity(v) === 'warning').length})</option>
                  <option value="info">Info ({stepValidations.filter(v => getSeverity(v) === 'info').length})</option>
                </select>
              </div>

              {/* Validation Results - minimal style */}
              {filteredValidations.length === 0 ? (
                <div className="text-sm text-gray-500 py-3">
                  No {validationFilter !== 'all' ? validationFilter : ''} validations found
                </div>
              ) : (
                <div className="space-y-2">
                  {filteredValidations.map((validation) => {
                    const severity = getSeverity(validation);

                    return (
                      <div key={validation.validationId} className="space-y-2">
                        {/* Individual Issues - Clean minimal style */}
                        {validation.validations.length > 0 ? (
                          validation.validations.map((issue, idx) => (
                            <div key={idx} className="border border-gray-200 rounded-md p-3 hover:border-gray-300 transition-colors">
                              <div className="flex items-start gap-2">
                                {severity === 'critical' && <AlertTriangle className="w-4 h-4 text-red-600 flex-shrink-0 mt-0.5" />}
                                {severity === 'warning' && <AlertTriangle className="w-4 h-4 text-yellow-600 flex-shrink-0 mt-0.5" />}
                                {severity === 'info' && <Info className="w-4 h-4 text-blue-600 flex-shrink-0 mt-0.5" />}
                                <div className="flex-1 min-w-0">
                                  <div className="text-sm text-gray-900 mb-1">{issue.message}</div>
                                  {issue.field && (
                                    <div className="text-xs text-gray-500 mb-1">
                                      <span className="font-medium">Field:</span> <span className="font-mono">{issue.field}</span>
                                    </div>
                                  )}
                                  {issue.expected && issue.actual && (
                                    <div className="text-xs text-gray-500 space-y-0.5">
                                      <div><span className="font-medium">Expected:</span> <span className="font-mono">{issue.expected}</span></div>
                                      <div><span className="font-medium">Actual:</span> <span className="font-mono">{issue.actual}</span></div>
                                    </div>
                                  )}
                                  <div className="text-xs text-gray-400 mt-2">
                                    {formatDistanceToNow(new Date(validation.timestamp), { addSuffix: true })}
                                  </div>
                                </div>
                              </div>
                            </div>
                          ))
                        ) : (
                          <div className="border border-gray-200 rounded-md p-3">
                            <div className="flex items-start gap-2">
                              <Info className="w-4 h-4 text-blue-600 flex-shrink-0 mt-0.5" />
                              <div className="flex-1">
                                <div className="text-sm text-gray-900">Validation completed</div>
                                <div className="text-xs text-gray-500 mt-1">
                                  Status: {validation.overallStatus}
                                </div>
                                <div className="text-xs text-gray-400 mt-1">
                                  {formatDistanceToNow(new Date(validation.timestamp), { addSuffix: true })}
                                </div>
                              </div>
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

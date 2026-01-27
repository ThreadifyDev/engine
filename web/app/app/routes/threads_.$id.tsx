import { useParams } from '@remix-run/react';
import { useQuery } from '@tanstack/react-query';
import { graphqlClient, type Thread, type StepStateInfo, type ValidationResultInfo } from '~/lib/graphql';
import { formatDistanceToNow } from 'date-fns';
import { 
  CheckCircle2, 
  XCircle, 
  Clock, 
  AlertTriangle, 
  Info,
  ChevronDown,
  ChevronRight,
  ExternalLink
} from 'lucide-react';
import { useState } from 'react';
import SideNav from '~/components/SideNav';
import ThreadGraphView from '~/components/ThreadGraphView';

type TabType = 'timeline' | 'graph';

export default function ThreadDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [activeTab, setActiveTab] = useState<TabType>('timeline');
  
  const { data: thread, isLoading, error } = useQuery({
    queryKey: ['thread', id],
    queryFn: () => graphqlClient.getThread(id!),
    enabled: !!id,
    refetchInterval: false, // We'll add real-time later
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="bg-red-50 border border-red-200 rounded-lg p-6 max-w-md">
          <h2 className="text-red-800 font-semibold mb-2">Error loading thread</h2>
          <p className="text-red-600">{(error as Error).message}</p>
        </div>
      </div>
    );
  }

  if (!thread) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="text-gray-500">Thread not found</div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-white flex">
      <SideNav />
      
      <main className="flex-1 ml-64 p-12">
        <ThreadHeader thread={thread} />
        
        {/* Tabs */}
        <div className="mt-8 border-b-2 border-black">
          <div className="flex gap-1">
            <button
              onClick={() => setActiveTab('timeline')}
              className={`px-6 py-3 font-medium transition-colors ${
                activeTab === 'timeline'
                  ? 'bg-black text-white'
                  : 'bg-white text-black hover:bg-gray-100'
              }`}
            >
              Timeline View
            </button>
            <button
              onClick={() => setActiveTab('graph')}
              className={`px-6 py-3 font-medium transition-colors ${
                activeTab === 'graph'
                  ? 'bg-black text-white'
                  : 'bg-white text-black hover:bg-gray-100'
              }`}
            >
              Graph View
            </button>
          </div>
        </div>
        
        {/* Tab Content */}
        <div className="mt-6">
          {activeTab === 'timeline' ? (
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-2 space-y-6">
                <StepTimeline steps={thread.steps || []} />
              </div>
              <div className="space-y-6">
                <ThreadOverview thread={thread} />
                <ValidationResults validations={thread.validationResults || []} />
              </div>
            </div>
          ) : (
            <div className="space-y-6">
              <ThreadGraphView 
                steps={thread.steps || []} 
                validations={thread.validationResults || []}
              />
              <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                <ThreadOverview thread={thread} />
                <ValidationResults validations={thread.validationResults || []} />
              </div>
            </div>
          )}
        </div>
      </main>
    </div>
  );
}

function ThreadHeader({ thread }: { thread: Thread }) {
  const statusConfig = {
    active: { color: 'bg-green-100 text-green-800', icon: '●', label: 'Active' },
    completed: { color: 'bg-blue-100 text-blue-800', icon: '✓', label: 'Completed' },
    failed: { color: 'bg-red-100 text-red-800', icon: '✗', label: 'Failed' },
    cancelled: { color: 'bg-gray-100 text-gray-800', icon: '○', label: 'Cancelled' },
  };

  const config = statusConfig[thread.status as keyof typeof statusConfig] || statusConfig.active;

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold text-gray-900">Thread: {thread.id}</h1>
            <span className={`px-3 py-1 rounded-full text-sm font-medium ${config.color}`}>
              {config.icon} {config.label}
            </span>
          </div>
          
          <div className="mt-4 space-y-2 text-sm text-gray-600">
            {thread.contractName && (
              <div className="flex items-center gap-2">
                <span className="font-medium">Contract:</span>
                <span>{thread.contractName}</span>
                {thread.contractVersion && (
                  <span className="text-gray-400">v{thread.contractVersion}</span>
                )}
              </div>
            )}
            
            {thread.startedAt && (
              <div className="flex items-center gap-2">
                <span className="font-medium">Started:</span>
                <span>{formatDistanceToNow(new Date(thread.startedAt), { addSuffix: true })}</span>
                <span className="text-gray-400">({new Date(thread.startedAt).toLocaleString()})</span>
              </div>
            )}
            
            {thread.completedAt && (
              <div className="flex items-center gap-2">
                <span className="font-medium">Completed:</span>
                <span>{formatDistanceToNow(new Date(thread.completedAt), { addSuffix: true })}</span>
              </div>
            )}

            {thread.error && (
              <div className="flex items-start gap-2 text-red-600">
                <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
                <span>{thread.error}</span>
              </div>
            )}
          </div>
        </div>
      </div>

      {thread.refs && Object.keys(thread.refs).length > 0 && (
        <div className="mt-6 pt-6 border-t border-gray-200">
          <h3 className="text-sm font-medium text-gray-700 mb-3 flex items-center gap-2">
            <ExternalLink className="w-4 h-4" />
            External References
          </h3>
          <div className="grid grid-cols-2 gap-3">
            {Object.entries(thread.refs).map(([key, value]) => (
              <div key={key} className="text-sm">
                <span className="text-gray-500">{key}:</span>{' '}
                <span className="font-mono text-gray-900">{String(value)}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function ThreadOverview({ thread }: { thread: Thread }) {
  const steps = thread.steps || [];
  const successCount = steps.filter(s => s.status === 'success').length;
  const failedCount = steps.filter(s => s.status === 'failed').length;
  const pendingCount = steps.filter(s => s.status === 'pending' || s.status === 'in_progress').length;

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
      <h2 className="text-lg font-semibold text-gray-900 mb-4">Overview</h2>
      
      <div className="grid grid-cols-2 gap-4">
        <div className="text-center p-4 bg-gray-50 rounded-lg">
          <div className="text-2xl font-bold text-gray-900">{steps.length}</div>
          <div className="text-sm text-gray-600">Total Steps</div>
        </div>
        
        <div className="text-center p-4 bg-green-50 rounded-lg">
          <div className="text-2xl font-bold text-green-700">{successCount}</div>
          <div className="text-sm text-gray-600">Success</div>
        </div>
        
        <div className="text-center p-4 bg-red-50 rounded-lg">
          <div className="text-2xl font-bold text-red-700">{failedCount}</div>
          <div className="text-sm text-gray-600">Failed</div>
        </div>
        
        <div className="text-center p-4 bg-yellow-50 rounded-lg">
          <div className="text-2xl font-bold text-yellow-700">{pendingCount}</div>
          <div className="text-sm text-gray-600">Pending</div>
        </div>
      </div>
    </div>
  );
}

function StepTimeline({ steps }: { steps: StepStateInfo[] }) {
  const [expandedSteps, setExpandedSteps] = useState<Set<string>>(new Set());

  const toggleStep = (stepId: string) => {
    setExpandedSteps(prev => {
      const next = new Set(prev);
      if (next.has(stepId)) {
        next.delete(stepId);
      } else {
        next.add(stepId);
      }
      return next;
    });
  };

  const getStepIcon = (status: string) => {
    switch (status) {
      case 'success':
        return <CheckCircle2 className="w-5 h-5 text-green-600" />;
      case 'failed':
        return <XCircle className="w-5 h-5 text-red-600" />;
      case 'in_progress':
        return <Clock className="w-5 h-5 text-blue-600 animate-pulse" />;
      default:
        return <Clock className="w-5 h-5 text-gray-400" />;
    }
  };

  const getStepBgColor = (status: string) => {
    switch (status) {
      case 'success':
        return 'bg-green-50 border-green-200';
      case 'failed':
        return 'bg-red-50 border-red-200';
      case 'in_progress':
        return 'bg-blue-50 border-blue-200';
      default:
        return 'bg-gray-50 border-gray-200';
    }
  };

  if (steps.length === 0) {
    return (
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-8 text-center">
        <Clock className="w-12 h-12 text-gray-400 mx-auto mb-3" />
        <p className="text-gray-500">No steps recorded yet</p>
      </div>
    );
  }

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
      <h2 className="text-lg font-semibold text-gray-900 mb-4">Step Timeline</h2>
      
      <div className="space-y-3">
        {steps.map((step, index) => {
          const stepId = `${step.stepName}:${step.idempotencyKey}`;
          const isExpanded = expandedSteps.has(stepId);
          
          return (
            <div
              key={stepId}
              className={`border rounded-lg p-4 transition-all ${getStepBgColor(step.status)}`}
            >
              <div className="flex items-start gap-3">
                <div className="flex-shrink-0 mt-0.5">
                  {getStepIcon(step.status)}
                </div>
                
                <div className="flex-1 min-w-0">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex-1">
                      <h3 className="font-semibold text-gray-900">{step.stepName}</h3>
                      <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-gray-600">
                        <span className="capitalize">Status: {step.status}</span>
                        {step.retryCount > 0 && (
                          <span className="text-orange-600">Retries: {step.retryCount}</span>
                        )}
                        <span>
                          {formatDistanceToNow(new Date(step.lastUpdatedAt), { addSuffix: true })}
                        </span>
                      </div>
                    </div>
                    
                    <button
                      onClick={() => toggleStep(stepId)}
                      className="flex-shrink-0 p-1 hover:bg-white/50 rounded transition-colors"
                    >
                      {isExpanded ? (
                        <ChevronDown className="w-5 h-5 text-gray-500" />
                      ) : (
                        <ChevronRight className="w-5 h-5 text-gray-500" />
                      )}
                    </button>
                  </div>
                  
                  {isExpanded && (
                    <div className="mt-4 pt-4 border-t border-gray-200 space-y-2 text-sm">
                      <div className="grid grid-cols-2 gap-2">
                        <div>
                          <span className="text-gray-500">Idempotency Key:</span>
                          <div className="font-mono text-xs text-gray-900 mt-1 break-all">
                            {step.idempotencyKey}
                          </div>
                        </div>
                        <div>
                          <span className="text-gray-500">Step ID:</span>
                          <div className="font-mono text-xs text-gray-900 mt-1 break-all">
                            {step.latestStepID}
                          </div>
                        </div>
                      </div>
                      
                      <div>
                        <span className="text-gray-500">First Seen:</span>
                        <div className="text-gray-900 mt-1">
                          {new Date(step.firstSeenAt).toLocaleString()}
                        </div>
                      </div>
                      
                      {step.previousStep && (
                        <div>
                          <span className="text-gray-500">Previous Step:</span>
                          <div className="text-gray-900 mt-1">{step.previousStep}</div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function ValidationResults({ validations }: { validations: ValidationResultInfo[] }) {
  const criticalViolations = validations.filter(v => v.hasCriticalViolation);
  const warnings = validations.filter(v => !v.hasCriticalViolation && v.warningCount > 0);

  if (validations.length === 0) {
    return (
      <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
        <h2 className="text-lg font-semibold text-gray-900 mb-4">Validations</h2>
        <div className="text-center py-4">
          <CheckCircle2 className="w-8 h-8 text-green-600 mx-auto mb-2" />
          <p className="text-sm text-gray-600">No validation issues</p>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-white rounded-lg shadow-sm border border-gray-200 p-6">
      <h2 className="text-lg font-semibold text-gray-900 mb-4 flex items-center gap-2">
        Validations
        {criticalViolations.length > 0 && (
          <span className="px-2 py-0.5 bg-red-100 text-red-800 text-xs font-medium rounded-full">
            {criticalViolations.length} critical
          </span>
        )}
      </h2>
      
      <div className="space-y-3">
        {validations.map((validation) => (
          <div
            key={validation.validationId}
            className={`border rounded-lg p-4 ${
              validation.hasCriticalViolation
                ? 'bg-red-50 border-red-200'
                : 'bg-yellow-50 border-yellow-200'
            }`}
          >
            <div className="flex items-start gap-2">
              {validation.hasCriticalViolation ? (
                <AlertTriangle className="w-5 h-5 text-red-600 flex-shrink-0 mt-0.5" />
              ) : (
                <Info className="w-5 h-5 text-yellow-600 flex-shrink-0 mt-0.5" />
              )}
              
              <div className="flex-1 min-w-0">
                <div className="font-medium text-gray-900 mb-1">
                  Step: {validation.stepName}
                </div>
                
                <div className="space-y-2">
                  {validation.validations.map((issue, idx) => (
                    <div key={idx} className="text-sm">
                      <div className="text-gray-900">{issue.message}</div>
                      {issue.field && (
                        <div className="text-gray-600 mt-1">
                          Field: <span className="font-mono">{issue.field}</span>
                        </div>
                      )}
                      {issue.expected && issue.actual && (
                        <div className="text-gray-600 mt-1">
                          Expected: {issue.expected} | Actual: {issue.actual}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
                
                <div className="mt-2 text-xs text-gray-500">
                  {formatDistanceToNow(new Date(validation.timestamp), { addSuffix: true })}
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

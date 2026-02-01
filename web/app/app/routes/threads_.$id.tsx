import { useParams } from '@remix-run/react';
import { useQuery } from '@tanstack/react-query';
import { graphqlClient, type Thread, type StepStateInfo, type ValidationResultInfo, type StepHistory } from '~/lib/graphql';
import { formatDistanceToNow } from 'date-fns';
import {
  CheckCircle2,
  XCircle,
  Clock,
  AlertTriangle,
  Calendar,
  Hash,
  Code,
  ChevronRight,
  RefreshCw,
  Users,
  Filter,
  ChevronDown,
  ChevronUp,
  Building2,
  User,
  ExternalLink,
  Copy,
  Check,
  Loader2,
  Shield,
  ShieldAlert,
  Info,
  X,
} from 'lucide-react';
import { useState } from 'react';
import SideNav from '~/components/SideNav';
import ThreadGraphView from '~/components/ThreadGraphViewReactFlow';
import RightSidebar from '~/components/RightSidebar';

type TabType = 'timeline' | 'graph';
type SidebarView = 'step' | 'participants' | 'validations' | 'stepValidations' | null;

// Helper function to calculate execution time from startedAt and finishedAt
function calculateExecutionTime(startedAt?: string, finishedAt?: string): string | null {
  if (!startedAt || !finishedAt) return null;
  
  const start = new Date(startedAt).getTime();
  const end = new Date(finishedAt).getTime();
  const durationMs = end - start;
  
  if (durationMs < 0) return null;
  if (durationMs < 1000) return `${durationMs}ms`;
  if (durationMs < 60000) return `${(durationMs / 1000).toFixed(2)}s`;
  return `${(durationMs / 60000).toFixed(2)}m`;
}

export default function ThreadDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [activeTab, setActiveTab] = useState<TabType>('graph');
  const [sidebarView, setSidebarView] = useState<SidebarView>(null);
  const [selectedStep, setSelectedStep] = useState<StepStateInfo | null>(null);
  const [showContext, setShowContext] = useState(false);
  const [previousView, setPreviousView] = useState<SidebarView>(null);
  const [severityFilter, setSeverityFilter] = useState<Set<string>>(new Set(['critical', 'warning', 'info']));
  
  const { data: thread, isLoading, error } = useQuery({
    queryKey: ['thread', id],
    queryFn: () => graphqlClient.getThread(id!),
    enabled: !!id,
  });

  // Fetch step history only when participants view is opened
  const { data: allStepHistory } = useQuery({
    queryKey: ['allStepHistoryLatest', id],
    queryFn: async () => {
      if (!thread?.steps || thread.steps.length === 0) return [];
      const historyPromises = thread.steps.map(step =>
        graphqlClient.getStepHistory(id!, step.stepName, step.idempotencyKey, 1)
      );
      const results = await Promise.all(historyPromises);
      return results.flat();
    },
    enabled: !!thread?.steps && thread.steps.length > 0 && sidebarView === 'participants',
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
      
      <main className="flex-1 ml-16 p-8 bg-gray-50">
        <div className="max-w-7xl mx-auto">
          <ThreadHeader thread={thread} />
          
          {/* Tabs */}
          <div className="mt-6 flex items-center justify-between">
            <div className="flex gap-2">
              <button
                onClick={() => setActiveTab('graph')}
                className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
                  activeTab === 'graph'
                    ? 'bg-white text-gray-900 shadow-sm border border-gray-200'
                    : 'text-gray-600 hover:text-gray-900 hover:bg-gray-100'
                }`}
              >
                Graph
              </button>
              {/* Timeline tab temporarily disabled */}
              {/* <button
                onClick={() => setActiveTab('timeline')}
                className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
                  activeTab === 'timeline'
                    ? 'bg-white text-gray-900 shadow-sm border border-gray-200'
                    : 'text-gray-600 hover:text-gray-900 hover:bg-gray-100'
                }`}
              >
                Timeline
              </button> */}
            </div>
            
            {/* Action Buttons */}
            <div className="flex gap-2">
              <div className="relative group">
                <button
                  onClick={() => {
                    if (thread.contractName) {
                      setSidebarView('validations');
                      setSelectedStep(null);
                      setPreviousView(null);
                    }
                  }}
                  disabled={!thread.contractName}
                  className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors flex items-center gap-2 ${
                    thread.contractName
                      ? 'text-gray-700 hover:text-gray-900 hover:bg-gray-100'
                      : 'text-gray-400 bg-gray-50 cursor-not-allowed'
                  }`}
                  title={!thread.contractName ? 'Thread is not attached to a contract' : ''}
                >
                  <AlertTriangle className="w-4 h-4" />
                  Flow Validations
                </button>
                {!thread.contractName && (
                  <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 px-3 py-2 bg-gray-900 text-white text-xs rounded-lg opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none whitespace-nowrap">
                    Thread is not attached to a contract
                    <div className="absolute top-full left-1/2 -translate-x-1/2 -mt-1 border-4 border-transparent border-t-gray-900"></div>
                  </div>
                )}
              </div>
              <button
                onClick={() => {
                  setSidebarView('participants');
                  setSelectedStep(null);
                  setPreviousView(null);
                }}
                className="px-4 py-2 text-sm font-medium text-gray-700 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors flex items-center gap-2"
              >
                <Users className="w-4 h-4" />
                Participants
              </button>
            </div>
          </div>
          
          {/* Tab Content */}
          <div className="mt-6">
            {/* {activeTab === 'timeline' ? (
              <CompactStepTimeline 
                steps={thread.steps || []} 
                currentCompanyId={thread.companyId}
                onStepClick={(step) => {
                  setSelectedStep(step);
                  setSidebarView('step');
                  setShowContext(false);
                }}
              />
            ) : ( */}
              <ThreadGraphView 
                steps={thread.steps || []} 
                validations={thread.validationResults || []}
                onNodeClick={(step: StepStateInfo) => {
                  setSelectedStep(step);
                  setSidebarView('step');
                  setShowContext(false);
                }}
              />
            {/* )} */}
          </div>
        </div>

        {/* Right Sidebar */}
        {sidebarView === 'step' && selectedStep && (
          <RightSidebar
            isOpen={true}
            onClose={() => {
              setSidebarView(null);
              setSelectedStep(null);
              setPreviousView(null);
            }}
            title="Step Details"
            width="lg"
          >
            <StepDetailContent 
              step={selectedStep} 
              threadId={id!}
              showContext={showContext}
              onToggleContext={() => setShowContext(!showContext)}
              validations={thread.validationResults || []}
            />
          </RightSidebar>
        )}

        {sidebarView === 'validations' && (
          <RightSidebar
            isOpen={true}
            onClose={() => {
              setSidebarView(null);
              setPreviousView(null);
            }}
            title="Validation Results"
            width="lg"
          >
            <ValidationResultsView 
              validations={thread.validationResults || []} 
              severityFilter={severityFilter}
              onSeverityFilterChange={setSeverityFilter}
            />
          </RightSidebar>
        )}

        {sidebarView === 'stepValidations' && selectedStep && (
          <RightSidebar
            isOpen={true}
            onClose={() => {
              setSidebarView(null);
              setSelectedStep(null);
              setPreviousView(null);
            }}
            title={
              <div className="flex items-center gap-3">
                <button
                  onClick={() => {
                    setSidebarView(previousView);
                    setPreviousView(null);
                  }}
                  className="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 transition-colors"
                >
                  <ChevronRight className="w-4 h-4 rotate-180" />
                  Back
                </button>
                <span className="text-gray-300">|</span>
                <span>Validations: {selectedStep.stepName}</span>
              </div>
            }
            width="lg"
          >
            <StepValidationResultsView 
              step={selectedStep}
              validations={thread.validationResults || []}
            />
          </RightSidebar>
        )}

        {sidebarView === 'participants' && (
          <RightSidebar
            isOpen={true}
            onClose={() => {
              setSidebarView(null);
              setPreviousView(null);
            }}
            title="Thread Participants"
            width="md"
          >
            <ParticipantsView threadId={id!} steps={thread.steps || []} stepHistory={allStepHistory} />
          </RightSidebar>
        )}
      </main>
    </div>
  );
}

function ThreadHeader({ thread }: { thread: Thread }) {
  const [copied, setCopied] = useState(false);
  const [copiedRef, setCopiedRef] = useState<string | null>(null);
  const steps = thread.steps || [];
  const successCount = steps.filter(s => s.status === 'success').length;
  const failedCount = steps.filter(s => s.status === 'failed').length;
  const pendingCount = steps.filter(s => s.status === 'pending' || s.status === 'in_progress').length;

  // Fetch thread hash chain verification
  const { data: hashChainStatus, isLoading: isVerifying } = useQuery({
    queryKey: ['threadIntegrity', thread.id],
    queryFn: () => graphqlClient.verifyThreadIntegrity(thread.id),
    enabled: !!thread.id && steps.length > 0,
    refetchInterval: false,
  });

  // Resolve owner name from ownerId
  const { data: resolvedOwner } = useQuery({
    queryKey: ['resolveOwner', thread.ownerId],
    queryFn: () => graphqlClient.resolveActors([thread.ownerId]),
    enabled: !!thread.ownerId,
  });

  const copyThreadId = () => {
    navigator.clipboard.writeText(thread.id);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const statusConfig = {
    active: { label: 'Active', color: 'bg-green-50 text-green-700 border-green-200' },
    completed: { label: 'Completed', color: 'bg-blue-50 text-blue-700 border-blue-200' },
    failed: { label: 'Failed', color: 'bg-red-50 text-red-700 border-red-200' },
    pending: { label: 'Pending', color: 'bg-gray-50 text-gray-700 border-gray-200' },
  };

  const config = statusConfig[thread.status as keyof typeof statusConfig] || statusConfig.active;

  return (
    <div className="border-b border-gray-200 pb-6">
      {/* Thread ID and Status */}
      <div className="flex items-center gap-3 mb-4">
        <h1 className="text-2xl font-semibold text-gray-900">{thread.id}</h1>
        <button
          onClick={copyThreadId}
          className="p-1.5 hover:bg-gray-100 rounded-lg transition-colors"
          title="Copy thread ID"
        >
          {copied ? <Check className="w-4 h-4 text-green-600" /> : <Copy className="w-4 h-4 text-gray-400" />}
        </button>
        <span className={`px-2.5 py-1 rounded-lg text-xs font-medium border ${config.color}`}>
          {config.label}
        </span>
        
        {/* Hash Chain Verification Badge */}
        {steps.length > 0 && (
          <div className="flex items-center">
            {isVerifying ? (
              <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-gray-50 border border-gray-200">
                <Loader2 className="w-3.5 h-3.5 text-gray-400 animate-spin" />
                <span className="text-xs text-gray-600">Verifying...</span>
              </div>
            ) : hashChainStatus ? (
              <div 
                className={`flex items-center gap-1.5 px-2.5 py-1 rounded-lg border cursor-help ${
                  hashChainStatus.verified 
                    ? 'bg-green-50 border-green-200' 
                    : 'bg-red-50 border-red-200'
                }`}
                title={hashChainStatus.verified 
                  ? `All ${hashChainStatus.totalEvents} steps verified - No tampering detected. Verified ${formatDistanceToNow(new Date(hashChainStatus.lastVerifiedAt), { addSuffix: true })}.`
                  : `Integrity check failed - Steps may be out of order or modified. ${hashChainStatus.error || ''}`}
              >
                {hashChainStatus.verified ? (
                  <>
                    <Shield className="w-3.5 h-3.5 text-green-600" />
                    <span className="text-xs font-medium text-green-700">Hash Verified</span>
                  </>
                ) : (
                  <>
                    <ShieldAlert className="w-3.5 h-3.5 text-red-600" />
                    <span className="text-xs font-medium text-red-700">Hash Broken</span>
                  </>
                )}
              </div>
            ) : null}
          </div>
        )}
      </div>

      {/* Metadata Row */}
      <div className="flex items-center gap-6 text-sm text-gray-600">
        {thread.startedAt && (
          <div className="flex items-center gap-2">
            <span className="text-gray-500">Started</span>
            <span className="font-medium text-gray-900">{formatDistanceToNow(new Date(thread.startedAt), { addSuffix: true })}</span>
          </div>
        )}
        
        {thread.ownerId && (
          <div className="flex items-center gap-2">
            <span className="text-gray-500">Created By</span>
            <span className="font-medium text-gray-900">
              {resolvedOwner?.[0]?.name || thread.ownerId}
            </span>
          </div>
        )}
        
        {thread.contractName && (
          <div className="flex items-center gap-2">
            <span className="text-gray-500">Contract</span>
            <span className="font-medium text-gray-900">{thread.contractName}</span>
            {thread.contractVersion && (
              <span className="text-gray-400">v{thread.contractVersion}</span>
            )}
          </div>
        )}

        <div className="flex items-center gap-4 ml-auto">
          <div className="flex items-center gap-1.5">
            <span className="text-gray-500">Steps</span>
            <span className="font-medium text-gray-900">{steps.length}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <div className="w-2 h-2 rounded-full bg-green-500"></div>
            <span className="font-medium text-gray-900">{successCount}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <div className="w-2 h-2 rounded-full bg-red-500"></div>
            <span className="font-medium text-gray-900">{failedCount}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <div className="w-2 h-2 rounded-full bg-yellow-500"></div>
            <span className="font-medium text-gray-900">{pendingCount}</span>
          </div>
        </div>
      </div>

      {/* External References */}
      <div className="mt-4 pt-4 border-t border-gray-100">
        {(() => {
          let refsObj: Record<string, any> | null = null;
          
          if (thread.refs) {
            // Parse refs if it's a JSON string
            if (typeof thread.refs === 'string') {
              try {
                refsObj = JSON.parse(thread.refs);
              } catch (e) {
                refsObj = null;
              }
            } else if (typeof thread.refs === 'object') {
              refsObj = thread.refs;
            }
          }
          
          const refEntries = refsObj ? Object.entries(refsObj).filter(([key]) => isNaN(Number(key))) : [];
          const hasRefs = refEntries.length > 0;
          
          const copyToClipboard = (value: string, refKey: string) => {
            navigator.clipboard.writeText(value);
            setCopiedRef(refKey);
            setTimeout(() => setCopiedRef(null), 2000);
          };
          
          return hasRefs ? (
            <>
              <div className="text-xs font-semibold text-gray-700 mb-2 flex items-center gap-2">
                <span>External References</span>
                <span className="text-xs bg-gray-200 text-gray-700 px-1.5 py-0.5 rounded-full">{refEntries.length}</span>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {refEntries.map(([key, value]) => (
                  <div
                    key={key}
                    className="inline-flex items-center gap-1.5 px-2 py-1 bg-blue-50 border border-blue-200 rounded-full text-xs"
                  >
                    <span className="font-medium text-blue-900">{key}:</span>
                    <span className="text-blue-700 font-mono text-xs truncate max-w-[120px]" title={String(value)}>
                      {String(value)}
                    </span>
                    <button
                      onClick={() => copyToClipboard(String(value), key)}
                      className="ml-1 p-0.5 hover:bg-blue-100 rounded transition-colors flex-shrink-0"
                      title="Copy value"
                    >
                      {copiedRef === key ? (
                        <Check className="w-3 h-3 text-green-600" />
                      ) : (
                        <Copy className="w-3 h-3 text-blue-600 hover:text-blue-800" />
                      )}
                    </button>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <p className="text-sm text-gray-400">No external system referenced</p>
          );
        })()}
      </div>

      {thread.error && (
        <div className="mt-4 flex items-start gap-2 text-sm text-red-600 bg-red-50 border border-red-200 rounded-md p-3">
          <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
          <span>{thread.error}</span>
        </div>
      )}
    </div>
  );
}


function CompactStepTimeline({ steps, onStepClick, currentCompanyId }: { steps: StepStateInfo[]; onStepClick: (step: StepStateInfo) => void; currentCompanyId?: string }) {
  const getStepIcon = (status: string) => {
    switch (status) {
      case 'success':
        return <CheckCircle2 className="w-4 h-4 text-green-600" />;
      case 'failed':
        return <XCircle className="w-4 h-4 text-red-600" />;
      case 'in_progress':
        return <Clock className="w-4 h-4 text-blue-600 animate-pulse" />;
      default:
        return <Clock className="w-4 h-4 text-gray-400" />;
    }
  };

  const getStepBgColor = (status: string) => {
    switch (status) {
      case 'success':
        return 'bg-green-50 border-green-200 hover:bg-green-100';
      case 'failed':
        return 'bg-red-50 border-red-200 hover:bg-red-100';
      case 'in_progress':
        return 'bg-blue-50 border-blue-200 hover:bg-blue-100';
      default:
        return 'bg-gray-50 border-gray-200 hover:bg-gray-100';
    }
  };

  // Generate consistent color from company ID (Stripe-style light pastels)
  const getCompanyColor = (companyId: string) => {
    // Simple hash function
    let hash = 0;
    for (let i = 0; i < companyId.length; i++) {
      hash = ((hash << 5) - hash) + companyId.charCodeAt(i);
      hash = hash & hash; // Convert to 32bit integer
    }
    
    // Stripe-style pastel colors (light, subtle, professional)
    const colors = [
      { border: 'border-purple-200', bg: 'bg-purple-50', text: 'text-purple-700' },
      { border: 'border-blue-200', bg: 'bg-blue-50', text: 'text-blue-700' },
      { border: 'border-cyan-200', bg: 'bg-cyan-50', text: 'text-cyan-700' },
      { border: 'border-teal-200', bg: 'bg-teal-50', text: 'text-teal-700' },
      { border: 'border-emerald-200', bg: 'bg-emerald-50', text: 'text-emerald-700' },
      { border: 'border-amber-200', bg: 'bg-amber-50', text: 'text-amber-700' },
      { border: 'border-orange-200', bg: 'bg-orange-50', text: 'text-orange-700' },
      { border: 'border-pink-200', bg: 'bg-pink-50', text: 'text-pink-700' },
      { border: 'border-rose-200', bg: 'bg-rose-50', text: 'text-rose-700' },
      { border: 'border-indigo-200', bg: 'bg-indigo-50', text: 'text-indigo-700' },
    ];
    
    return colors[Math.abs(hash) % colors.length];
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
      
      <div className="space-y-2">
        <StepTimelineWithGrouping
          steps={steps}
          currentCompanyId={currentCompanyId}
          onStepClick={onStepClick}
          getStepIcon={getStepIcon}
          getStepBgColor={getStepBgColor}
          getCompanyColor={getCompanyColor}
        />
      </div>
    </div>
  );
}

// Component to handle grouping logic with step history data
function StepTimelineWithGrouping({
  steps,
  currentCompanyId,
  onStepClick,
  getStepIcon,
  getStepBgColor,
  getCompanyColor,
  stepHistory,
}: {
  steps: StepStateInfo[];
  currentCompanyId?: string;
  onStepClick: (step: StepStateInfo) => void;
  getStepIcon: (status: string) => JSX.Element;
  getStepBgColor: (status: string) => string;
  getCompanyColor: (companyId: string) => { border: string; bg: string; text: string };
  stepHistory?: any[];
}) {
  if (!stepHistory) {
    return (
      <>
        {steps.map((step) => (
          <StepTimelineItem 
            key={`${step.stepName}:${step.idempotencyKey}`} 
            step={step} 
            onStepClick={onStepClick} 
            getStepIcon={getStepIcon} 
            getStepBgColor={getStepBgColor}
            currentCompanyId={currentCompanyId}
            companyColor={undefined}
            showCompanyLabel={false}
          />
        ))}
      </>
    );
  }

  // Group consecutive steps by company
  type StepGroup = {
    companyId: string;
    companyName: string;
    steps: Array<{ step: StepStateInfo; history: any }>;
  };

  const groups: StepGroup[] = [];
  
  steps.forEach((step, index) => {
    const history = stepHistory.find(h => h.actor === step.stepName) || stepHistory[index];
    const companyId = history?.companyId || currentCompanyId || '';
    const companyName = history?.companyName || '';
    
    const lastGroup = groups[groups.length - 1];
    
    if (lastGroup && lastGroup.companyId === companyId) {
      lastGroup.steps.push({ step, history });
    } else {
      groups.push({
        companyId,
        companyName,
        steps: [{ step, history }],
      });
    }
  });

  return (
    <>
      {groups.map((group, groupIndex) => {
        const isExternal = group.companyId !== currentCompanyId;
        const companyColor = isExternal ? getCompanyColor(group.companyId) : null;
        
        return (
          <div key={groupIndex}>
            {isExternal && (
              <div className={`border-2 ${companyColor?.border} rounded-lg p-3 ${companyColor?.bg} space-y-2`}>
                <div className={`text-xs font-medium mb-2 px-1 flex items-center gap-1 ${companyColor?.text}`}>
                  <ExternalLink className="w-3 h-3" />
                  <span>External: {group.companyName}</span>
                </div>
                {group.steps.map(({ step }) => (
                  <StepTimelineItem 
                    key={`${step.stepName}:${step.idempotencyKey}`} 
                    step={step} 
                    onStepClick={onStepClick} 
                    getStepIcon={getStepIcon} 
                    getStepBgColor={getStepBgColor}
                    currentCompanyId={currentCompanyId}
                    companyColor={companyColor}
                    showCompanyLabel={false}
                  />
                ))}
              </div>
            )}
            {!isExternal && group.steps.map(({ step }) => (
              <StepTimelineItem 
                key={`${step.stepName}:${step.idempotencyKey}`} 
                step={step} 
                onStepClick={onStepClick} 
                getStepIcon={getStepIcon} 
                getStepBgColor={getStepBgColor}
                currentCompanyId={currentCompanyId}
                companyColor={null}
                showCompanyLabel={false}
              />
            ))}
          </div>
        );
      })}
    </>
  );
}

function StepTimelineItem({ 
  step, 
  onStepClick, 
  getStepIcon, 
  getStepBgColor,
  currentCompanyId,
  companyColor,
  showCompanyLabel,
}: { 
  step: StepStateInfo; 
  onStepClick: (step: StepStateInfo) => void;
  getStepIcon: (status: string) => JSX.Element;
  getStepBgColor: (status: string) => string;
  currentCompanyId?: string;
  companyColor?: { border: string; bg: string; text: string } | null;
  showCompanyLabel: boolean;
}) {
  // Fetch latest step history to get actor/service info
  const { data: history } = useQuery({
    queryKey: ['stepHistoryLatest', step.threadId, step.stepName, step.idempotencyKey],
    queryFn: () => graphqlClient.getStepHistory(step.threadId, step.stepName, step.idempotencyKey, 1),
  });

  const latestHistory = history?.[0];

  return (
    <button
      onClick={() => onStepClick(step)}
      className={`w-full border rounded-lg p-3 transition-all text-left ${getStepBgColor(step.status)}`}
    >
      <div className="flex items-center gap-3">
        <div className="flex-shrink-0">
          {getStepIcon(step.status)}
        </div>
        
        <div className="flex-1 min-w-0">
          <h3 className="font-semibold text-gray-900 text-sm">{step.stepName}</h3>
          <div className="mt-0.5 flex items-center gap-3 text-xs text-gray-600">
            <span className="capitalize">{step.status}</span>
            {step.retryCount > 0 && (
              <span className="text-orange-600 flex items-center gap-1">
                <RefreshCw className="w-3 h-3" />
                {step.retryCount}
              </span>
            )}
            <span>
              {formatDistanceToNow(new Date(step.lastUpdatedAt), { addSuffix: true })}
            </span>
          </div>
          {latestHistory && (
            <div className="mt-1 flex items-center gap-2 text-xs text-gray-500">
              {latestHistory.actorService && (
                <span className="flex items-center gap-1">
                  <Code className="w-3 h-3" />
                  {latestHistory.actorService}
                </span>
              )}
              {latestHistory.actor && (
                <span className="flex items-center gap-1">
                  <Users className="w-3 h-3" />
                  {latestHistory.actor}
                </span>
              )}
            </div>
          )}
        </div>
        
        <ChevronRight className="w-4 h-4 text-gray-400 flex-shrink-0" />
      </div>
    </button>
  );
}

function StepDetailContent({ 
  step, 
  threadId, 
  showContext, 
  onToggleContext,
  validations
}: { 
  step: StepStateInfo; 
  threadId: string; 
  showContext: boolean; 
  onToggleContext: () => void;
  validations: ValidationResultInfo[];
}) {
  const [showValidations, setShowValidations] = useState(false);
  const [validationFilter, setValidationFilter] = useState<'all' | 'critical' | 'warning' | 'info'>('all');
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

  const [showHistory, setShowHistory] = useState(false);

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
        
        <span className={`inline-flex px-2 py-0.5 text-xs font-medium rounded-full ${
          step.status === 'success' ? 'bg-green-100 text-green-800' :
          step.status === 'failed' ? 'bg-red-100 text-red-800' :
          step.status === 'in_progress' ? 'bg-blue-100 text-blue-800' :
          'bg-gray-100 text-gray-800'
        }`}>
          {step.status}
        </span>
      </div>

      {/* Retry Count */}
      {step.retryCount > 0 && (
        <div className="bg-orange-50 border-2 border-orange-200 p-4 rounded">
          <div className="flex items-center gap-2 text-orange-800">
            <RefreshCw className="w-5 h-5" />
            <span className="font-bold">Retried {step.retryCount} time{step.retryCount > 1 ? 's' : ''}</span>
          </div>
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
              <div className="text-xs text-gray-500">{new Date(step.firstSeenAt).toLocaleString()}</div>
            </div>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-600">Last Updated:</span>
            <div className="text-right">
              <div className="font-medium">{formatDistanceToNow(new Date(step.lastUpdatedAt), { addSuffix: true })}</div>
              <div className="text-xs text-gray-500">{new Date(step.lastUpdatedAt).toLocaleString()}</div>
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

      {/* Context Data Toggle - Stripe style */}
      <div className="border-t border-gray-200 pt-6">
        <button
          onClick={onToggleContext}
          className="w-full px-3 py-3 text-left transition-colors group hover:bg-gray-50"
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
          <div className="mt-3 space-y-3">
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
                </button>              </div>
            </div>
          </div>
        )}

        {showContext && !step.latestContext && (
          <div className="mt-3 text-sm text-gray-500 italic">
            No context data available
          </div>
        )}
      </div>

      {/* View History - Separate section */}
      <div className="border-t border-gray-200 pt-6">
        <button
          onClick={() => setShowHistory(true)}
          className="w-full px-3 py-3 text-left transition-colors group hover:bg-gray-50 rounded-lg"
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
              {/* Filter Dropdown - Stripe style */}
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

              {/* Validation Results - Stripe style */}
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
                        {/* Individual Issues - Clean Stripe style */}
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

      {/* Nested History Sidebar - Stripe pattern */}
      {showHistory && (
        <StepHistorySidebar
          step={step}
          threadId={threadId}
          onClose={() => setShowHistory(false)}
        />
      )}
    </div>
  );
}

// Nested History Sidebar Component
function StepHistorySidebar({
  step,
  threadId,
  onClose,
}: {
  step: StepStateInfo;
  threadId: string;
  onClose: () => void;
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
    <>
      {/* Overlay */}
      <div
        className="fixed inset-0 bg-black/20 z-40"
        onClick={onClose}
      />

      {/* Sidebar */}
      <div className="fixed top-0 right-0 h-full w-[345px] bg-white shadow-2xl z-50 flex flex-col animate-slide-in-right">
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-gray-200">
          <div>
            <h3 className="font-semibold text-gray-900">Step History</h3>
            <p className="text-xs text-gray-500 mt-0.5">
              {step.stepName}
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded hover:bg-gray-100 transition-colors"
          >
            <X className="w-5 h-5 text-gray-500" />
          </button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4">
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

                        {/* Error */}
                        {item.error && (
                          <div>
                            <div className="text-xs font-medium text-red-600 mb-1">Error</div>
                            <div className="text-xs bg-red-50 text-red-900 p-2 rounded border border-red-200">
                              {item.error}
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
      </div>
    </>
  );
}

// Actor Section Component - Resolves actor ID to name
function ActorSection({ actorId, actorService }: { actorId: string; actorService?: string }) {
  const { data: actors, isLoading } = useQuery({
    queryKey: ['resolveActor', actorId],
    queryFn: () => graphqlClient.resolveActors([actorId]),
  });

  const actor = actors?.[0];

  return (
    <div className="space-y-3">
      <h5 className="font-bold text-gray-900 flex items-center gap-2">
        <User className="w-4 h-4" />
        Actor
      </h5>
      {isLoading ? (
        <div className="text-sm text-gray-500">Loading actor information...</div>
      ) : actor ? (
        <div className="bg-gray-50 border border-gray-200 rounded-md p-3">
          <div className="flex items-start justify-between">
            <div className="flex-1">
              <div className="font-medium text-sm text-gray-900">{actor.name}</div>
              {actor.email && (
                <div className="text-xs text-gray-500 mt-1">{actor.email}</div>
              )}
              {actor.companyName && (
                <div className="text-xs text-gray-500 mt-1">{actor.companyName}</div>
              )}
              {actorService && (
                <div className="text-xs text-blue-600 mt-1 font-mono">Service: {actorService}</div>
              )}
            </div>
            <span className={`text-xs px-2 py-1 rounded-full ${
              actor.type === 'user' 
                ? 'bg-blue-100 text-blue-700' 
                : 'bg-purple-100 text-purple-700'
            }`}>
              {actor.type === 'user' ? 'User' : 'Service Account'}
            </span>
          </div>
        </div>
      ) : (
        <div className="text-sm text-gray-500">
          <div className="font-mono text-xs bg-gray-100 p-2 rounded">
            {actorId}
          </div>
          {actorService && (
            <div className="text-xs text-blue-600 mt-2 font-mono">Service: {actorService}</div>
          )}
        </div>
      )}
    </div>
  );
}

function ParticipantsView({ threadId, steps, stepHistory }: { threadId: string; steps: StepStateInfo[]; stepHistory?: any[] }) {
  // Extract unique services and actor IDs from StepHistory objects
  const services = new Set<string>();
  const actorIds = new Set<string>();
  
  stepHistory?.forEach(item => {
    if (item.actorService) services.add(item.actorService);
    if (item.actor) actorIds.add(item.actor);
  });

  // Fetch resolved actor names - MUST be called before any conditional returns
  const { data: resolvedActors, isLoading: isLoadingActors } = useQuery({
    queryKey: ['resolveActors', Array.from(actorIds)],
    queryFn: () => graphqlClient.resolveActors(Array.from(actorIds)),
    enabled: actorIds.size > 0,
  });

  if (!stepHistory) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-gray-600"></div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Services */}
      <div>
        <h5 className="font-semibold text-gray-900 mb-3 flex items-center gap-2 text-sm">
          <Code className="w-4 h-4" />
          Services ({services.size})
        </h5>
        {services.size > 0 ? (
          <div className="space-y-2">
            {Array.from(services).map(service => (
              <div key={service} className="bg-gray-50 border border-gray-200 rounded-md p-3">
                <div className="font-mono text-sm text-gray-900">{service}</div>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-gray-500">No service information available</p>
        )}
      </div>

      {/* Actors/Service Accounts */}
      <div>
        <h5 className="font-semibold text-gray-900 mb-3 flex items-center gap-2 text-sm">
          <Users className="w-4 h-4" />
          Actors ({actorIds.size})
        </h5>
        {isLoadingActors ? (
          <p className="text-sm text-gray-500">Loading actor information...</p>
        ) : actorIds.size > 0 && resolvedActors ? (
          <div className="space-y-2">
            {resolvedActors.map(actor => (
              <div key={actor.id} className="bg-gray-50 border border-gray-200 rounded-md p-3">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="font-medium text-sm text-gray-900">{actor.name}</div>
                    {actor.companyName && (
                      <div className="text-xs text-gray-500 mt-1">{actor.companyName}</div>
                    )}
                  </div>
                  <span className={`text-xs px-2 py-1 rounded-full ${
                    actor.type === 'user' 
                      ? 'bg-blue-100 text-blue-700' 
                      : 'bg-purple-100 text-purple-700'
                  }`}>
                    {actor.type === 'user' ? 'User' : 'Service Account'}
                  </span>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-gray-500">No actor information available</p>
        )}
      </div>

      {/* Step Count */}
      <div className="border-t border-gray-200 pt-6">
        <div className="bg-gray-900 text-white p-4 rounded-md">
          <div className="text-2xl font-semibold">{steps.length}</div>
          <div className="text-sm text-gray-300">Total Steps</div>
        </div>
      </div>
    </div>
  );
}

function ValidationResultsView({ 
  validations,
  severityFilter,
  onSeverityFilterChange
}: { 
  validations: ValidationResultInfo[];
  severityFilter: Set<string>;
  onSeverityFilterChange: (filter: Set<string>) => void;
}) {
  // Determine severity for each validation
  const getSeverity = (validation: ValidationResultInfo): string => {
    if (validation.hasCriticalViolation || validation.criticalCount > 0) return 'critical';
    if (validation.warningCount > 0) return 'warning';
    return 'info';
  };

  const criticalCount = validations.filter(v => getSeverity(v) === 'critical').length;
  const warningCount = validations.filter(v => getSeverity(v) === 'warning').length;
  const infoCount = validations.filter(v => getSeverity(v) === 'info').length;

  // Filter validations by severity
  const filteredValidations = validations.filter(v => severityFilter.has(getSeverity(v)));

  const toggleSeverity = (severity: string) => {
    const newFilter = new Set(severityFilter);
    if (newFilter.has(severity)) {
      newFilter.delete(severity);
    } else {
      newFilter.add(severity);
    }
    onSeverityFilterChange(newFilter);
  };

  if (validations.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <CheckCircle2 className="w-16 h-16 text-green-600 mb-4" />
        <h3 className="text-lg font-semibold text-gray-900 mb-2">No Validation Issues</h3>
        <p className="text-sm text-gray-600">All steps passed validation checks</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Filter Buttons */}
      <div className="flex gap-2">
        <button
          onClick={() => toggleSeverity('critical')}
          className={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
            severityFilter.has('critical')
              ? 'bg-red-100 text-red-700 border-2 border-red-300'
              : 'bg-gray-100 text-gray-400 border-2 border-gray-200'
          }`}
        >
          Critical ({criticalCount})
        </button>
        <button
          onClick={() => toggleSeverity('warning')}
          className={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
            severityFilter.has('warning')
              ? 'bg-yellow-100 text-yellow-700 border-2 border-yellow-300'
              : 'bg-gray-100 text-gray-400 border-2 border-gray-200'
          }`}
        >
          Warning ({warningCount})
        </button>
        <button
          onClick={() => toggleSeverity('info')}
          className={`px-3 py-1.5 text-xs font-medium rounded-md transition-colors ${
            severityFilter.has('info')
              ? 'bg-blue-100 text-blue-700 border-2 border-blue-300'
              : 'bg-gray-100 text-gray-400 border-2 border-gray-200'
          }`}
        >
          Info ({infoCount})
        </button>
      </div>

      {/* Validation List */}
      <div className="space-y-3">
        {filteredValidations.map((validation) => {
          const severity = getSeverity(validation);
          const getSeverityStyles = () => {
            switch (severity) {
              case 'critical':
                return {
                  bg: 'bg-red-50',
                  border: 'border-red-200',
                  icon: <AlertTriangle className="w-5 h-5 text-red-600 flex-shrink-0 mt-0.5" />
                };
              case 'warning':
                return {
                  bg: 'bg-yellow-50',
                  border: 'border-yellow-200',
                  icon: <AlertTriangle className="w-5 h-5 text-yellow-600 flex-shrink-0 mt-0.5" />
                };
              case 'info':
              default:
                return {
                  bg: 'bg-blue-50',
                  border: 'border-blue-200',
                  icon: <Info className="w-5 h-5 text-blue-600 flex-shrink-0 mt-0.5" />
                };
            }
          };
          const styles = getSeverityStyles();
          
          return (
            <div
              key={validation.validationId}
              className={`border-2 rounded-md p-4 ${styles.bg} ${styles.border}`}
            >
              <div className="flex items-start gap-3">
                {styles.icon}
              
              <div className="flex-1 min-w-0">
                <div className="font-semibold text-gray-900 mb-1">
                  {validation.stepName}
                </div>
                <div className="text-xs text-gray-500 mb-2">
                  {validation.idempotencyKey}
                </div>
                
                <div className="space-y-2">
                  {validation.validations.map((issue, idx) => (
                    <div key={idx} className="text-sm">
                      <div className="text-gray-900 font-medium">{issue.message}</div>
                      {issue.field && (
                        <div className="text-gray-600 mt-1 text-xs">
                          Field: <span className="font-mono">{issue.field}</span>
                        </div>
                      )}
                      {issue.expected && issue.actual && (
                        <div className="text-gray-600 mt-1 text-xs">
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
        );
        })}
      </div>
    </div>
  );
}

function StepValidationResultsView({ 
  step, 
  validations
}: { 
  step: StepStateInfo; 
  validations: ValidationResultInfo[];
}) {
  const stepValidations = validations.filter(
    v => v.stepName === step.stepName && v.idempotencyKey === step.idempotencyKey
  );

  // Determine severity for each validation
  const getSeverity = (validation: ValidationResultInfo): string => {
    if (validation.hasCriticalViolation || validation.criticalCount > 0) return 'critical';
    if (validation.warningCount > 0) return 'warning';
    return 'info';
  };

  const getStatusBadge = (validation: ValidationResultInfo) => {
    const severity = getSeverity(validation);
    
    if (validation.overallStatus === 'success') {
      return (
        <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-green-100 text-green-700 text-xs font-medium">
          <CheckCircle2 className="w-3 h-3" />
          success
        </div>
      );
    }
    
    switch (severity) {
      case 'critical':
        return (
          <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-red-100 text-red-700 text-xs font-medium">
            <AlertTriangle className="w-3 h-3" />
            critical
          </div>
        );
      case 'warning':
        return (
          <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-yellow-100 text-yellow-700 text-xs font-medium">
            <AlertTriangle className="w-3 h-3" />
            warning
          </div>
        );
      default:
        return (
          <div className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full bg-blue-100 text-blue-700 text-xs font-medium">
            <Info className="w-3 h-3" />
            info
          </div>
        );
    }
  };

  return (
    <div className="space-y-6">
      {/* Step Header - Matching Step Details style */}
      <div>
        <h2 className="text-2xl font-bold text-gray-900 mb-2">{step.stepName}</h2>
        {stepValidations.length > 0 && (
          <div className="mb-3">
            {getStatusBadge(stepValidations[0])}
          </div>
        )}
      </div>

      {/* Timeline Section - Matching Step Details style */}
      <div className="border-t border-gray-200 pt-6">
        <div className="flex items-center gap-2 mb-4">
          <Calendar className="w-4 h-4 text-gray-700" />
          <h3 className="text-sm font-semibold text-gray-900">Timeline</h3>
        </div>
        
        <div className="space-y-3 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-600">Validated:</span>
            <div className="text-right">
              <div className="text-gray-900">
                {formatDistanceToNow(new Date(step.lastUpdatedAt), { addSuffix: true })}
              </div>
              <div className="text-xs text-gray-500">
                {new Date(step.lastUpdatedAt).toLocaleString('en-US', {
                  day: '2-digit',
                  month: '2-digit',
                  year: 'numeric',
                  hour: '2-digit',
                  minute: '2-digit',
                  second: '2-digit'
                })}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Identifiers Section - Matching Step Details style */}
      <div className="border-t border-gray-200 pt-6">
        <div className="flex items-center gap-2 mb-4">
          <Hash className="w-4 h-4 text-gray-700" />
          <h3 className="text-sm font-semibold text-gray-900">Identifiers</h3>
        </div>
        
        <div className="space-y-3">
          <div>
            <div className="text-xs text-gray-600 mb-1">Idempotency Key:</div>
            <div className="bg-gray-50 border border-gray-200 rounded px-3 py-2 text-sm font-mono text-gray-900">
              {step.idempotencyKey}
            </div>
          </div>
        </div>
      </div>

      {/* Validation Results */}
      {stepValidations.length === 0 ? (
        <div className="border-t border-gray-200 pt-6">
          <div className="flex flex-col items-center justify-center py-8">
            <CheckCircle2 className="w-12 h-12 text-green-600 mb-3" />
            <h3 className="text-base font-semibold text-gray-900 mb-1">All Validations Passed</h3>
            <p className="text-sm text-gray-600">This step has no validation issues</p>
          </div>
        </div>
      ) : (
        <div className="border-t border-gray-200 pt-6">
          <h3 className="text-sm font-semibold text-gray-900 mb-4">Validation Details</h3>
          
          <div className="space-y-4">
            {stepValidations.map((validation) => (
              <div key={validation.validationId} className="space-y-3">
                {/* Summary Stats */}
                {validation.totalValidations > 0 && (
                  <div className="flex gap-4 text-xs">
                    {validation.criticalCount > 0 && (
                      <div className="flex items-center gap-1 text-red-700">
                        <AlertTriangle className="w-3 h-3" />
                        <span className="font-medium">{validation.criticalCount} critical</span>
                      </div>
                    )}
                    {validation.warningCount > 0 && (
                      <div className="flex items-center gap-1 text-yellow-700">
                        <AlertTriangle className="w-3 h-3" />
                        <span className="font-medium">{validation.warningCount} warnings</span>
                      </div>
                    )}
                    {validation.infoCount > 0 && (
                      <div className="flex items-center gap-1 text-blue-700">
                        <Info className="w-3 h-3" />
                        <span className="font-medium">{validation.infoCount} info</span>
                      </div>
                    )}
                  </div>
                )}

                {/* Individual Issues */}
                {validation.validations.length > 0 ? (
                  <div className="space-y-2">
                    {validation.validations.map((issue, idx) => (
                      <div key={idx} className="bg-gray-50 border border-gray-200 rounded-lg p-3">
                        <div className="text-sm text-gray-900 font-medium mb-1">{issue.message}</div>
                        {issue.field && (
                          <div className="text-xs text-gray-600 mb-1">
                            Field: <span className="font-mono bg-white px-1.5 py-0.5 rounded border border-gray-200">{issue.field}</span>
                          </div>
                        )}
                        {issue.expected && issue.actual && (
                          <div className="text-xs text-gray-600 space-y-0.5">
                            <div>Expected: <span className="font-mono">{issue.expected}</span></div>
                            <div>Actual: <span className="font-mono">{issue.actual}</span></div>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="bg-blue-50 border border-blue-200 rounded-lg p-3">
                    <div className="flex items-start gap-2">
                      <Info className="w-4 h-4 text-blue-600 flex-shrink-0 mt-0.5" />
                      <div className="text-sm text-blue-900">
                        Validation completed with status: <span className="font-medium">{validation.overallStatus}</span>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

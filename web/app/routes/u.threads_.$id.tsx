import { TabBar } from '~/components/TabBar';
import { useParams, useNavigate } from 'react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { graphqlClient, type Thread, type StepStateInfo, type ValidationResultInfo, type StepHistory, type ThreadNotification, type NotificationSummary } from '~/lib/graphql';
import { getConfig } from '~/config.client';

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
import { useEffect, useState } from 'react';
import AppLayout from '~/components/AppLayout';
import ThreadGraphView from '~/components/ThreadGraphView';
import ThreadTimelineView from '~/components/ThreadTimelineView';
import GanttTimelineView from '~/components/GanttTimelineView';
import RightSidebar from '~/components/RightSidebar';
import { ThreadHeader } from '~/components/thread/ThreadHeader';
import { StepDetailContent } from '~/components/thread/StepDetailContent';
import { ValidationResultsView } from '~/components/thread/ValidationResultsView';
import { ParticipantsView } from '~/components/thread/ParticipantsView';
import { StepHistoryContent } from '~/components/thread/StepHistoryContent';
import { SubStateSidebar } from '~/components/thread/SubStateSidebar';
import { CompactStepTimeline } from '~/components/thread/StepTimeline';
import { StepValidationResultsView } from '~/components/thread/StepValidationResultsView';
import { NotificationDetailView } from '~/components/thread/NotificationDetailView';

type TabType = 'timeline' | 'graph' | 'gantt';
type SidebarView = 'step' | 'participants' | 'validations' | 'step-validations' | 'step-violations' | null;

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
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [liveConnected, setLiveConnected] = useState(false);
  const [activeTab, setActiveTab] = useState<TabType>('timeline');
  const [sidebarView, setSidebarView] = useState<SidebarView>(null);
  const [selectedStep, setSelectedStep] = useState<StepStateInfo | null>(null);
  const [showContext, setShowContext] = useState(false);
  const [previousView, setPreviousView] = useState<SidebarView>(null);
  const [severityFilter, setSeverityFilter] = useState<Set<string>>(new Set(['critical', 'major', 'warning', 'minor', 'info', 'unclassified']));
  const [selectedNotification, setSelectedNotification] = useState<ThreadNotification | null>(null);
  const [selectedStepForHistory, setSelectedStepForHistory] = useState<StepStateInfo | null>(null);
  const [selectedStepForViolations, setSelectedStepForViolations] = useState<StepStateInfo | null>(null);
  const [selectedSubSteps, setSelectedSubSteps] = useState<{subSteps: any[], stepName: string, stepStartedAt?: string} | null>(null);
  const [selectedService, setSelectedService] = useState<string | null>(null);
  
  const { data: thread, isLoading, error, isRefetching } = useQuery({
    queryKey: ['thread', id],
    queryFn: () => graphqlClient.getThread(id!),
    enabled: !!id,
    staleTime: 0,
    refetchOnMount: true,
    refetchOnWindowFocus: false,
  });

  const isLive = thread?.status === 'active';
  useEffect(() => {
    if (!id || !isLive) return;
    let source: EventSource | null = null;
    let refreshTimer: number | undefined;
    let reconcileTimer: number | undefined;
    let fullRefresh = false;
    let refreshNotifications = false;
    let refreshHistory = false;
    const changedSteps = new Map<string, { stepName: string; idempotencyKey: string }>();
    const reconcileSteps = new Map<string, { stepName: string; idempotencyKey: string }>();
    const loadChangedSteps = async (steps: { stepName: string; idempotencyKey: string }[]) => {
      try {
        const updates = await Promise.all(steps.map(step =>
          graphqlClient.getThreadStep(id, step.stepName, step.idempotencyKey)
        ));
        if (updates.some(step => !step)) {
          void queryClient.invalidateQueries({ queryKey: ['thread', id] });
          return;
        }
        queryClient.setQueryData<Thread>(['thread', id], previous => {
          if (!previous) return previous;
          const nextSteps = [...(previous.steps ?? [])];
          for (const step of updates as StepStateInfo[]) {
            const index = nextSteps.findIndex(existing =>
              existing.stepName === step.stepName && existing.idempotencyKey === step.idempotencyKey
            );
            if (index < 0) nextSteps.push(step);
            else if (new Date(step.lastUpdatedAt).getTime() >= new Date(nextSteps[index].lastUpdatedAt).getTime()) nextSteps[index] = step;
          }
          return { ...previous, steps: nextSteps };
        });
      } catch {
        void queryClient.invalidateQueries({ queryKey: ['thread', id] });
      }
    };
    const refresh = async () => {
      refreshTimer = undefined;
      const steps = [...changedSteps.values()];
      changedSteps.clear();
      const notificationsChanged = refreshNotifications;
      const historyChanged = refreshHistory;
      refreshNotifications = false;
      refreshHistory = false;
      if (fullRefresh || steps.length > 3) {
        fullRefresh = false;
        void queryClient.invalidateQueries({ queryKey: ['thread', id] });
      } else if (steps.length > 0) {
        await loadChangedSteps(steps);
      }
      if (notificationsChanged) {
        void queryClient.invalidateQueries({ queryKey: ['threadNotifications', id] });
        void queryClient.invalidateQueries({ queryKey: ['stepViolations', id] });
      }
      if (historyChanged) void queryClient.invalidateQueries({ queryKey: ['stepHistory', id] });
    };
    const scheduleRefresh = () => {
      if (refreshTimer === undefined) refreshTimer = window.setTimeout(() => { void refresh(); }, 2000);
    };
    const onReady = () => {
      setLiveConnected(true);
      fullRefresh = true;
      scheduleRefresh(); // catch changes between the initial query and subscription
    };
    const onUpdate = (event: Event) => {
      try {
        const update = JSON.parse((event as MessageEvent).data) as { type?: string; stepName?: string; idempotencyKey?: string };
        if (update.type === 'state.step' && update.stepName && update.idempotencyKey !== undefined) {
          refreshHistory = true;
          if (reconcileTimer === undefined) {
            // The archiver writes richer step details asynchronously. Recheck
            // changed steps once after its batch window.
            reconcileTimer = window.setTimeout(() => {
              reconcileTimer = undefined;
              const steps = [...reconcileSteps.values()];
              reconcileSteps.clear();
              if (steps.length > 3) void queryClient.invalidateQueries({ queryKey: ['thread', id] });
              else if (steps.length > 0) void loadChangedSteps(steps);
            }, 10000);
          }
          const key = JSON.stringify([update.stepName, update.idempotencyKey]);
          const step = {
            stepName: update.stepName,
            idempotencyKey: update.idempotencyKey,
          };
          changedSteps.set(key, step);
          reconcileSteps.set(key, step);
        } else {
          fullRefresh = true;
          refreshNotifications ||= update.type === 'notifications.thread' || update.type === 'validations.thread' || update.type === 'resync';
          refreshHistory ||= update.type === 'resync';
        }
      } catch {
        fullRefresh = true;
        refreshNotifications = true;
        refreshHistory = true;
      }
      scheduleRefresh();
    };
    const disconnect = () => {
      source?.close();
      source = null;
      if (refreshTimer !== undefined) window.clearTimeout(refreshTimer);
      if (reconcileTimer !== undefined) window.clearTimeout(reconcileTimer);
      refreshTimer = undefined;
      reconcileTimer = undefined;
      changedSteps.clear();
      reconcileSteps.clear();
      fullRefresh = false;
      refreshNotifications = false;
      refreshHistory = false;
      setLiveConnected(false);
    };
    const connect = () => {
      if (document.hidden || source) return;
      source = new EventSource(`${getConfig().engineUrl.replace(/\/+$/, '')}/v1/threads/${encodeURIComponent(id)}/events`, { withCredentials: true });
      source.addEventListener('ready', onReady);
      source.addEventListener('update', onUpdate);
      source.onerror = () => setLiveConnected(false);
    };
    const onVisibilityChange = () => {
      if (document.hidden) disconnect();
      else connect();
    };
    document.addEventListener('visibilitychange', onVisibilityChange);
    connect();
    return () => {
      document.removeEventListener('visibilitychange', onVisibilityChange);
      disconnect();
    };
  }, [id, isLive, queryClient]);
  const currentStep = thread?.steps?.find(step =>
    step.stepName === selectedStep?.stepName && step.idempotencyKey === selectedStep?.idempotencyKey
  ) ?? selectedStep;
  const currentStepForHistory = thread?.steps?.find(step =>
    step.stepName === selectedStepForHistory?.stepName && step.idempotencyKey === selectedStepForHistory?.idempotencyKey
  ) ?? selectedStepForHistory;
  const currentStepForViolations = thread?.steps?.find(step =>
    step.stepName === selectedStepForViolations?.stepName && step.idempotencyKey === selectedStepForViolations?.idempotencyKey
  ) ?? selectedStepForViolations;


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

  // Fetch full notifications only when validations view is opened
  const { data: notifications, isLoading: notificationsLoading } = useQuery({
    queryKey: ['threadNotifications', id],
    queryFn: () => graphqlClient.getThreadNotifications(id!, {
      limit: 100,
    }),
    enabled: !!id && sidebarView === 'validations',
    refetchOnWindowFocus: true,
  });

  // Fetch step-specific violations when violation history view is opened
  const { data: stepViolations, isLoading: stepViolationsLoading } = useQuery({
    queryKey: ['stepViolations', id, selectedStepForViolations?.stepName, selectedStepForViolations?.idempotencyKey],
    queryFn: async () => {
      const result = await graphqlClient.getThreadNotifications(id!, {
        stepName: selectedStepForViolations!.stepName,
        // Don't filter by source - get all notifications for this step
        limit: 100,
      });
      return result;
    },
    enabled: !!id && !!selectedStepForViolations && sidebarView === 'step-violations',
    refetchOnWindowFocus: true,
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
      </div>
    );
  }

  if (error && !thread) {
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
    <AppLayout>
      <div className="min-h-screen min-w-0 w-full overflow-x-hidden bg-gray-50 p-4 sm:p-6 lg:p-8">
        <div className="max-w-7xl mx-auto">
          <ThreadHeader thread={thread} liveConnected={liveConnected} isRefetching={isRefetching} />
          
          {/* Tabs */}
          <div className="relative mt-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div aria-hidden="true" className="pointer-events-none absolute inset-0 rounded-xl border border-stone-200 bg-white shadow-sm" />
            <TabBar label="Thread view" value={activeTab} onChange={setActiveTab} panelId="thread-panel" surface={false} className="relative w-full p-1.5 sm:flex-1"
              items={[{value:'timeline',label:'Timeline'}]} />

            {/* Action Buttons */}
            <div className="relative flex flex-wrap gap-2">
              <div className="relative group flex-1 sm:flex-none">
                <button
                  onClick={() => {
                    if (sidebarView === 'validations') {
                      setSidebarView(null);
                      setPreviousView(null);
                    } else {
                      setSidebarView('validations');
                      setSelectedStep(null);
                      setPreviousView(null);
                    }
                  }}
                  disabled={!thread.contractName}
                  className={`w-full min-w-0 justify-center px-3 py-2 text-sm font-medium rounded-lg transition-colors flex items-center gap-2 sm:w-auto sm:px-4 ${
                    thread.contractName
                      ? 'text-gray-700 hover:text-gray-900 hover:bg-gray-100'
                      : 'text-gray-400 cursor-not-allowed'
                  }`}
                  title={!thread.contractName ? 'Thread is not attached to a contract' : ''}
                >
                  <AlertTriangle className="w-4 h-4" />
                  Contract Validations
                  {thread.notificationSummary && (thread.notificationSummary.hasCritical || thread.notificationSummary.hasWarnings) && (
                    <span className="ml-1 px-2 py-0.5 text-xs font-semibold rounded-full bg-red-100 text-red-700">
                      {thread.notificationSummary.criticalCount + thread.notificationSummary.warningCount}
                    </span>
                  )}
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
                className="min-w-0 flex-1 justify-center px-3 py-2 text-sm font-medium text-gray-700 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors flex items-center gap-2 sm:flex-none sm:px-4"
              >
                <Users className="w-4 h-4" />
                Participants
              </button>
            </div>
          </div>
          
          {/* Tab Content */}
          <div id="thread-panel" role="tabpanel" aria-label="Thread content" className="mt-6">
            {activeTab === 'timeline' && (
              // <ThreadTimelineView 
              //   steps={thread.steps || []} 
              //   threadStatus={thread.status}
              //   onStepClick={(step) => {
              //     setSelectedStep(step);
              //     setSidebarView('step');
              //   }}
              // />
              <GanttTimelineView
                steps={thread.steps || []}
                threadStatus={thread.status}
                selectedService={selectedService}
                onServiceSelect={setSelectedService}
                onStepClick={(step) => {
                  setSelectedStep(step);
                  setSidebarView('step');
                }}
              />
            )}
            
            {activeTab === 'graph' && thread.contractName && (
              <ThreadGraphView 
                steps={thread.steps || []}
                contractName={thread.contractName}
                contractVersion={thread.contractVersion}
                onNodeClick={(step: StepStateInfo) => {
                  setSelectedStep(step);
                  setSidebarView('step');
                }}
              />
            )}
            
            {activeTab === 'graph' && !thread.contractName && (
              <div className="flex items-center justify-center h-96 text-gray-500 border-2 border-gray-200 rounded-lg bg-gray-50">
                <div className="text-center max-w-md">
                  <p className="text-lg font-medium mb-2">Graph view is only available for threads with contracts</p>
                  <p className="text-sm text-gray-400">
                    Contracts define the flow structure that powers the graph visualization.
                  </p>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Right Sidebar */}
        {sidebarView === 'step' && currentStep && (
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
              step={currentStep}
              threadId={id!}
              showContext={showContext}
              onToggleContext={() => setShowContext(!showContext)}
              validations={thread.validationResults || []}
              onShowHistory={(step) => setSelectedStepForHistory(step)}
              onShowSubState={(subSteps, stepName, stepStartedAt) => {
                setSelectedSubSteps({subSteps, stepName, stepStartedAt});
              }}
              onShowViolations={(step) => {
                setSelectedStepForViolations(step);
                setPreviousView('step');
                setSidebarView('step-violations');
              }}
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
              notifications={notifications || []}
              notificationsLoading={notificationsLoading}
              notificationSummary={thread.notificationSummary}
              severityFilter={severityFilter}
              onSeverityFilterChange={setSeverityFilter}
              onNotificationClick={(notif) => setSelectedNotification(notif)}
            />
          </RightSidebar>
        )}

        {/* Notification Detail Sidebar */}
        {selectedNotification && (
          <RightSidebar
            isOpen={true}
            onClose={() => {
              setSelectedNotification(null);
              // Restore violation history if we came from there
              if (previousView === 'step-violations' && selectedStepForViolations) {
                setSidebarView('step-violations');
                setPreviousView(null);
              }
            }}
            title={
              <div className="flex items-center gap-3">
                <button
                  onClick={() => {
                    setSelectedNotification(null);
                    // Restore violation history if we came from there
                    if (previousView === 'step-violations' && selectedStepForViolations) {
                      setSidebarView('step-violations');
                      setPreviousView(null);
                    }
                  }}
                  className="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 transition-colors"
                >
                  <ChevronRight className="w-4 h-4 rotate-180" />
                  Back
                </button>
                <span className="text-gray-300">|</span>
                <span>Notification Details</span>
              </div>
            }
            width="lg"
          >
            <NotificationDetailView notification={selectedNotification} />
          </RightSidebar>
        )}

        {/* Step History Layered Sidebar */}
        {currentStepForHistory && (
          <RightSidebar
            isOpen={true}
            onClose={() => setSelectedStepForHistory(null)}
            title={
              <div className="flex items-center gap-3">
                <button
                  onClick={() => setSelectedStepForHistory(null)}
                  className="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 transition-colors"
                >
                  <ChevronRight className="w-4 h-4 rotate-180" />
                  Back
                </button>
                <span className="text-gray-300">|</span>
                <span>Step History</span>
              </div>
            }
            width="lg"
          >
            <StepHistoryContent 
              step={currentStepForHistory}
              threadId={id!}
            />
          </RightSidebar>
        )}

        {sidebarView === 'step-validations' && currentStep && (
          <RightSidebar
            isOpen={true}
            onClose={() => {
              setSidebarView(null);
              setPreviousView(null);
              setSelectedStep(null);
            }}
            title={
              <div className="flex items-center gap-2">
                <button
                  onClick={() => {
                    setSelectedStep(currentStep);
                    setSidebarView(previousView);
                    setPreviousView(null);
                  }}
                  className="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 transition-colors"
                >
                  <ChevronRight className="w-4 h-4 rotate-180" />
                  Back
                </button>
                <span className="text-gray-300">|</span>
                <span>Validations: {currentStep.stepName}</span>
              </div>
            }
            width="lg"
          >
            <StepValidationResultsView 
              step={currentStep}
              validations={thread.validationResults || []}
            />
          </RightSidebar>
        )}

        {sidebarView === 'step-violations' && currentStepForViolations && (
          <RightSidebar
            isOpen={true}
            onClose={() => {
              setSidebarView(null);
              setPreviousView(null);
              setSelectedStepForViolations(null);
            }}
            title={
              <div className="flex items-center gap-2 min-w-0">
                <button
                  onClick={() => {
                    setSidebarView('step');
                    setPreviousView(null);
                    setSelectedStepForViolations(null);
                  }}
                  className="flex items-center gap-1 text-sm text-gray-600 hover:text-gray-900 transition-colors flex-shrink-0"
                >
                  <ChevronRight className="w-4 h-4 rotate-180" />
                  Back
                </button>
                <span className="text-gray-300 flex-shrink-0">|</span>
                <span className="truncate" title={`Violation History: ${currentStepForViolations.stepName}`}>
                  Violation History: {currentStepForViolations.stepName}
                </span>
              </div>
            }
            width="lg"
          >
            <ValidationResultsView 
              notifications={stepViolations || []}
              notificationsLoading={stepViolationsLoading}
              notificationSummary={undefined}
              severityFilter={new Set(['critical', 'major', 'warning', 'minor', 'info', 'unclassified'])}
              onSeverityFilterChange={() => {}}
              onNotificationClick={(notif) => {
                setSelectedNotification(notif);
                // Close violation history sidebar to prevent z-index stacking
                setPreviousView('step-violations');
                setSidebarView(null);
              }}
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
            <ParticipantsView
              threadId={id!}
              steps={thread.steps || []}
              stepHistory={allStepHistory}
              selectedService={selectedService}
              onServiceSelect={(service) => {
                setSelectedService(service);
                setSidebarView(null);
              }}
            />
          </RightSidebar>
        )}

        {/* Sub State Sidebar - Nested overlay */}
        {selectedSubSteps && (
          <SubStateSidebar
            subSteps={selectedSubSteps.subSteps}
            stepName={selectedSubSteps.stepName}
            stepStartedAt={selectedSubSteps.stepStartedAt}
            onClose={() => setSelectedSubSteps(null)}
            onBack={() => setSelectedSubSteps(null)}
          />
        )}
      </div>
    </AppLayout>
  );
}

import { useParams, useNavigate } from '@remix-run/react';
import type { MetaFunction } from "@remix-run/node";
import { useQuery } from '@tanstack/react-query';
import { graphqlClient, type Thread, type StepStateInfo, type ValidationResultInfo, type StepHistory, type ThreadNotification, type NotificationSummary } from '~/lib/graphql';

export const meta: MetaFunction = () => {
  return [
    { title: "Thread Details - Threadify" },
    { name: "description", content: "View thread execution details" },
  ];
};
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
type SidebarView = 'step' | 'participants' | 'validations' | 'stepValidations' | 'step-violations' | null;

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
  const [activeTab, setActiveTab] = useState<TabType>('timeline');
  const [sidebarView, setSidebarView] = useState<SidebarView>(null);
  const [selectedStep, setSelectedStep] = useState<StepStateInfo | null>(null);
  const [showContext, setShowContext] = useState(false);
  const [previousView, setPreviousView] = useState<SidebarView>(null);
  const [severityFilter, setSeverityFilter] = useState<Set<string>>(new Set(['critical', 'warning', 'info']));
  const [selectedNotification, setSelectedNotification] = useState<ThreadNotification | null>(null);
  const [selectedStepForHistory, setSelectedStepForHistory] = useState<StepStateInfo | null>(null);
  const [selectedStepForViolations, setSelectedStepForViolations] = useState<StepStateInfo | null>(null);
  const [selectedSubSteps, setSelectedSubSteps] = useState<{subSteps: any[], stepName: string, stepStartedAt?: string} | null>(null);
  
  const { data: thread, isLoading, error } = useQuery({
    queryKey: ['thread', id],
    queryFn: () => graphqlClient.getThread(id!),
    enabled: !!id,
    staleTime: 0, // Always fetch fresh data for thread details
    refetchOnMount: true, // Refetch when component mounts
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

  // Fetch full notifications only when validations view is opened
  const { data: notifications, isLoading: notificationsLoading } = useQuery({
    queryKey: ['threadNotifications', id, severityFilter],
    queryFn: () => graphqlClient.getThreadNotifications(id!, {
      severity: Array.from(severityFilter),
      limit: 100,
    }),
    enabled: !!id && sidebarView === 'validations',
  });

  // Fetch step-specific violations when violation history view is opened
  const { data: stepViolations, isLoading: stepViolationsLoading } = useQuery({
    queryKey: ['stepViolations', id, selectedStepForViolations?.stepName, selectedStepForViolations?.idempotencyKey],
    queryFn: async () => {
      const result = await graphqlClient.getThreadNotifications(id!, {
        stepName: selectedStepForViolations!.stepName,
        // Don't filter by source - get all notifications for this step
        severity: ['critical', 'warning'],
        limit: 100,
      });
      return result;
    },
    enabled: !!id && !!selectedStepForViolations && sidebarView === 'step-violations',
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
                onClick={() => setActiveTab('timeline')}
                className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors ${
                  activeTab === 'timeline'
                    ? 'bg-white text-gray-900 shadow-sm border border-gray-200'
                    : 'text-gray-600 hover:text-gray-900 hover:bg-gray-100'
                }`}
              >
                Timeline
              </button>
              
              {/* Only show Graph tab if thread has a contract */}
              {/* {thread.contractName && (
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
              )} */}
            </div>
            
            {/* Action Buttons */}
            <div className="flex gap-2">
              <div className="relative group">
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
                  className={`px-4 py-2 text-sm font-medium rounded-lg transition-colors flex items-center gap-2 ${
                    thread.contractName
                      ? 'text-gray-700 hover:text-gray-900 hover:bg-gray-100'
                      : 'text-gray-400 bg-gray-50 cursor-not-allowed'
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
                className="px-4 py-2 text-sm font-medium text-gray-700 hover:text-gray-900 hover:bg-gray-100 rounded-lg transition-colors flex items-center gap-2"
              >
                <Users className="w-4 h-4" />
                Participants
              </button>
            </div>
          </div>
          
          {/* Tab Content */}
          <div className="mt-6">
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
        {selectedStepForHistory && (
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
              step={selectedStepForHistory}
              threadId={id!}
            />
          </RightSidebar>
        )}

        {sidebarView === 'step-validations' && selectedStep && (
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
                    setSelectedStep(selectedStep);
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

        {sidebarView === 'step-violations' && selectedStepForViolations && (
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
                <span className="truncate" title={`Violation History: ${selectedStepForViolations.stepName}`}>
                  Violation History: {selectedStepForViolations.stepName}
                </span>
              </div>
            }
            width="lg"
          >
            <ValidationResultsView 
              notifications={stepViolations || []}
              notificationsLoading={stepViolationsLoading}
              notificationSummary={undefined}
              severityFilter={new Set(['critical', 'warning'])}
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
            <ParticipantsView threadId={id!} steps={thread.steps || []} stepHistory={allStepHistory} />
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
      </main>
    </div>
  );
}


import { useState, useMemo } from 'react';
import { CheckCircle2, XCircle, Clock, AlertTriangle, ChevronDown, ChevronRight, User } from 'lucide-react';
import type { StepStateInfo, SubStep as GraphQLSubStep } from '~/lib/graphql';

interface ThreadTimelineViewProps {
  steps: StepStateInfo[];
  onStepClick: (step: StepStateInfo) => void;
  threadStatus?: string;
}

interface ServiceGroup {
  service: string;
  steps: StepStateInfo[];
}

// Helper to calculate duration
const calculateDuration = (startTime?: string, endTime?: string): string | null => {
  if (!startTime || !endTime) return null;
  const start = new Date(startTime).getTime();
  const end = new Date(endTime).getTime();
  const durationMs = end - start;
  
  if (durationMs < 0) return null;
  if (durationMs < 1000) return `${durationMs}ms`;
  if (durationMs < 60000) return `${(durationMs / 1000).toFixed(2)}s`;
  if (durationMs < 3600000) return `${(durationMs / 60000).toFixed(2)}m`;
  return `${(durationMs / 3600000).toFixed(2)}h`;
};

const SERVICE_COLORS = [
  { bg: '#dbeafe', border: '#3b82f6', text: '#1e40af' }, // blue
  { bg: '#fce7f3', border: '#ec4899', text: '#9f1239' }, // pink
  { bg: '#dcfce7', border: '#22c55e', text: '#166534' }, // green
  { bg: '#fef3c7', border: '#f59e0b', text: '#92400e' }, // amber
  { bg: '#e0e7ff', border: '#6366f1', text: '#3730a3' }, // indigo
  { bg: '#fce4ec', border: '#f06292', text: '#880e4f' }, // rose
  { bg: '#e1f5fe', border: '#03a9f4', text: '#01579b' }, // light blue
  { bg: '#f3e5f5', border: '#9c27b0', text: '#4a148c' }, // purple
];

const getServiceColor = (serviceName: string) => {
  if (!serviceName) return SERVICE_COLORS[0];
  let hash = 0;
  for (let i = 0; i < serviceName.length; i++) {
    hash = ((hash << 5) - hash) + serviceName.charCodeAt(i);
    hash = hash & hash;
  }
  return SERVICE_COLORS[Math.abs(hash) % SERVICE_COLORS.length];
};

const getStatusIcon = (status: string) => {
  switch (status) {
    case 'success':
      return <CheckCircle2 className="w-5 h-5 text-green-600" />;
    case 'failed':
      return <XCircle className="w-5 h-5 text-red-600" />;
    case 'in_progress':
      return <Clock className="w-5 h-5 text-blue-600 animate-pulse" />;
    default:
      return <AlertTriangle className="w-5 h-5 text-yellow-600" />;
  }
};

const getStatusBadge = (status: string) => {
  switch (status) {
    case 'success':
      return 'bg-green-100 text-green-700 border-green-200';
    case 'failed':
      return 'bg-red-100 text-red-700 border-red-200';
    case 'in_progress':
      return 'bg-blue-100 text-blue-700 border-blue-200';
    default:
      return 'bg-yellow-100 text-yellow-700 border-yellow-200';
  }
};

function SubStepItem({ subStep }: { subStep: GraphQLSubStep }) {
  const [showPayload, setShowPayload] = useState(false);
  const duration = calculateDuration(subStep.createdAt, subStep.recordedAt);
  
  return (
    <div className="py-2 border-b border-gray-100 last:border-b-0">
      <div className="flex items-center gap-2 text-sm">
        {subStep.status === 'success' ? (
          <CheckCircle2 className="w-3.5 h-3.5 text-green-600 flex-shrink-0" />
        ) : (
          <XCircle className="w-3.5 h-3.5 text-red-600 flex-shrink-0" />
        )}
        <span className="flex-1 font-medium text-gray-700">{subStep.substepName}</span>
        <div className="flex items-center gap-2 text-xs text-gray-400">
          {duration && <span className="font-mono text-gray-500">{duration}</span>}
          <span>
            {new Date(subStep.recordedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
          </span>
        </div>
      </div>
      
      {/* Payload/Context */}
      {subStep.payload && (
        <div className="mt-1.5 ml-6">
          <button
            onClick={() => setShowPayload(!showPayload)}
            className="text-xs text-blue-600 hover:text-blue-800 flex items-center gap-1"
          >
            {showPayload ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
            Context
          </button>
          {showPayload && (
            <pre className="mt-1.5 p-2 bg-gray-50 rounded text-xs overflow-x-auto max-h-32 border border-gray-200">
              {typeof subStep.payload === 'string' 
                ? JSON.stringify(JSON.parse(subStep.payload), null, 2)
                : JSON.stringify(subStep.payload, null, 2)
              }
            </pre>
          )}
        </div>
      )}
    </div>
  );
}

function StepCard({ step, onClick }: { step: StepStateInfo; onClick: () => void }) {
  const [expanded, setExpanded] = useState(false);
  const hasSubSteps = step.subSteps && step.subSteps.length > 0;
  const duration = calculateDuration(step.firstSeenAt, step.lastUpdatedAt);

  return (
    <div className="relative mb-3">
      {/* Timeline dot and line */}
      <div className="absolute left-0 top-0 bottom-0 w-px bg-gray-200" />
      <div className="absolute left-0 top-[22px] w-2 h-2 rounded-full bg-blue-500 -ml-[3px]" />
      
      {/* Card */}
      <div className="ml-6">
        <div
          onClick={onClick}
          className="bg-white rounded-md border border-gray-200 hover:border-blue-400 hover:shadow-sm transition-all cursor-pointer p-3"
        >
          {/* Main row */}
          <div className="flex items-center gap-3">
            {/* Expand button on the left (like service groups) */}
            {hasSubSteps ? (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  setExpanded(!expanded);
                }}
                className="text-gray-600 hover:text-gray-900 flex-shrink-0"
              >
                {expanded ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
              </button>
            ) : (
              <div className="w-4" /> // Spacer for alignment
            )}
            
            {getStatusIcon(step.status)}
            
            <div className="flex-1 min-w-0">
              <div className="font-medium text-gray-900">{step.stepName}</div>
              <div className="text-xs text-gray-500 flex items-center gap-1">
                <User className="w-3 h-3" />
                {step.actorService || step.actor || 'Unknown'}
              </div>
            </div>
            
            {/* Metadata badges */}
            <div className="flex items-center gap-2 text-xs">
              {duration && (
                <span className="font-mono text-gray-500">{duration}</span>
              )}
              <span className="text-gray-400 flex items-center gap-1">
                <Clock className="w-3 h-3" />
                {new Date(step.lastUpdatedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
              </span>
              
              {hasSubSteps && (
                <span className="text-blue-600">
                  {step.subSteps!.length} sub-step{step.subSteps!.length !== 1 ? 's' : ''}
                </span>
              )}
              
              {step.retryCount > 1 && (
                <span className="text-blue-600 font-medium">⟳ {step.retryCount}</span>
              )}
              
              <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${getStatusBadge(step.status)}`}>
                {step.status}
              </span>
            </div>
          </div>
        </div>

        {/* Expanded Sub-Steps */}
        {expanded && hasSubSteps && (
          <div className="mt-2 ml-8 bg-gray-50 rounded-md border border-gray-200 p-3">
            <div className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">
              Sub-Steps
            </div>
            <div className="space-y-0">
              {step.subSteps!.map((subStep) => (
                <SubStepItem key={subStep.id} subStep={subStep} />
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function ServiceGroup({ group, onStepClick }: { group: ServiceGroup; onStepClick: (step: StepStateInfo) => void }) {
  const [collapsed, setCollapsed] = useState(false);
  const serviceColor = getServiceColor(group.service);

  return (
    <div className="mb-4">
      {/* Service Header */}
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="w-full flex items-center gap-2 px-3 py-2 rounded-md border hover:shadow-sm transition-all mb-2"
        style={{ 
          backgroundColor: serviceColor.bg,
          borderColor: serviceColor.border,
        }}
      >
        {collapsed ? <ChevronRight className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        <span className="font-semibold text-sm" style={{ color: serviceColor.text }}>
          {group.service}
        </span>
        <span className="text-xs text-gray-500">
          ({group.steps.length} step{group.steps.length !== 1 ? 's' : ''})
        </span>
      </button>

      {/* Steps */}
      {!collapsed && (
        <div className="pl-3">
          {group.steps.map((step) => (
            <StepCard
              key={step.latestStepID}
              step={step}
              onClick={() => onStepClick(step)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export default function ThreadTimelineView({ steps, onStepClick, threadStatus }: ThreadTimelineViewProps) {
  // Group steps by service
  const serviceGroups = useMemo(() => {
    const groups = new Map<string, StepStateInfo[]>();
    
    // Sort steps by timestamp first
    const sortedSteps = [...steps].sort((a, b) => {
      const timeA = new Date(a.firstSeenAt).getTime();
      const timeB = new Date(b.firstSeenAt).getTime();
      return timeA - timeB;
    });

    sortedSteps.forEach((step) => {
      const service = step.actorService || step.actor || 'Unknown Service';
      if (!groups.has(service)) {
        groups.set(service, []);
      }
      groups.get(service)!.push(step);
    });

    // Convert to array and maintain order
    const result: ServiceGroup[] = [];
    const seenServices = new Set<string>();
    
    sortedSteps.forEach((step) => {
      const service = step.actorService || step.actor || 'Unknown Service';
      if (!seenServices.has(service)) {
        seenServices.add(service);
        result.push({
          service,
          steps: groups.get(service)!,
        });
      }
    });

    return result;
  }, [steps]);

  if (steps.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-500">
        No steps to display
      </div>
    );
  }

  return (
    <div className="w-full h-full overflow-y-auto bg-gray-50 p-6">
      <div className="max-w-4xl mx-auto">
        {/* Header */}
        <div className="mb-6">
          <h2 className="text-2xl font-bold text-gray-900">Thread Timeline</h2>
          <p className="text-sm text-gray-600 mt-1">
            {steps.length} step{steps.length !== 1 ? 's' : ''} across {serviceGroups.length} service{serviceGroups.length !== 1 ? 's' : ''}
          </p>
        </div>

        {/* Timeline */}
        <div className="space-y-6">
          {serviceGroups.map((group) => (
            <ServiceGroup
              key={group.service}
              group={group}
              onStepClick={onStepClick}
            />
          ))}
          
          {/* Thread End Indicator */}
          <div className="ml-6 mt-4">
            <div className="flex items-center gap-3">
              {/* Pulsating dot for active threads */}
              {threadStatus === 'active' ? (
                <div className="relative -ml-[30px]">
                  <div className="w-3 h-3 rounded-full bg-blue-500 animate-pulse" />
                  <div className="absolute inset-0 w-3 h-3 rounded-full bg-blue-400 animate-ping opacity-75" />
                </div>
              ) : (
                <div className="w-3 h-3 rounded-full bg-gray-400 -ml-[30px]" />
              )}
              
              {/* Status Label */}
              <div className={`px-2.5 py-1 rounded-full text-xs font-medium ${
                threadStatus === 'active' 
                  ? 'bg-blue-100 text-blue-700 border border-blue-200' 
                  : threadStatus === 'completed'
                  ? 'bg-green-100 text-green-700 border border-green-200'
                  : threadStatus === 'cancelled'
                  ? 'bg-gray-100 text-gray-700 border border-gray-200'
                  : 'bg-yellow-100 text-yellow-700 border border-yellow-200'
              }`}>
                {threadStatus === 'active' && 'Active - Waiting for next step'}
                {threadStatus === 'completed' && 'Thread Completed'}
                {threadStatus === 'cancelled' && 'Thread Cancelled'}
                {!threadStatus && 'Unknown Status'}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

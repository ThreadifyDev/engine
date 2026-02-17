import { useQuery } from '@tanstack/react-query';
import { formatDistanceToNow } from 'date-fns';
import {
  CheckCircle2,
  XCircle,
  Clock,
  RefreshCw,
  Code,
  Users,
  ChevronRight,
  ExternalLink,
} from 'lucide-react';
import { graphqlClient, type StepStateInfo } from '~/lib/graphql';

export function CompactStepTimeline({ steps, onStepClick, currentCompanyId }: { steps: StepStateInfo[]; onStepClick: (step: StepStateInfo) => void; currentCompanyId?: string }) {
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

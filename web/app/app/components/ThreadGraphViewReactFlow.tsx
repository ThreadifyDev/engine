import { useCallback, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import ReactFlow, {
  Node,
  Edge,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  MarkerType,
  Handle,
  Position,
} from 'reactflow';
import 'reactflow/dist/style.css';
import { type StepStateInfo, graphqlClient } from '~/lib/graphql';
import { CheckCircle2, XCircle, Clock, RefreshCw, AlertTriangle, Link2, Link2Off, Shield, AlertCircle } from 'lucide-react';

interface ThreadGraphViewProps {
  steps: StepStateInfo[];
  validations?: any[];
  onNodeClick?: (step: StepStateInfo) => void;
}

// Service color palette - consistent colors for each service
const SERVICE_COLORS = [
  { border: '#8b5cf6', bg: '#f5f3ff', text: '#7c3aed', light: '#ede9fe' }, // violet
  { border: '#06b6d4', bg: '#ecfeff', text: '#0891b2', light: '#cffafe' }, // cyan
  { border: '#f59e0b', bg: '#fffbeb', text: '#d97706', light: '#fef3c7' }, // amber
  { border: '#ec4899', bg: '#fdf2f8', text: '#db2777', light: '#fce7f3' }, // pink
  { border: '#10b981', bg: '#ecfdf5', text: '#059669', light: '#d1fae5' }, // emerald
  { border: '#6366f1', bg: '#eef2ff', text: '#4f46e5', light: '#e0e7ff' }, // indigo
  { border: '#f97316', bg: '#fff7ed', text: '#ea580c', light: '#ffedd5' }, // orange
  { border: '#14b8a6', bg: '#f0fdfa', text: '#0d9488', light: '#ccfbf1' }, // teal
  { border: '#a855f7', bg: '#faf5ff', text: '#9333ea', light: '#f3e8ff' }, // purple
  { border: '#0ea5e9', bg: '#f0f9ff', text: '#0284c7', light: '#e0f2fe' }, // sky
];

// Generate consistent color from service name
const getServiceColor = (serviceName: string) => {
  if (!serviceName) return SERVICE_COLORS[0];
  let hash = 0;
  for (let i = 0; i < serviceName.length; i++) {
    hash = ((hash << 5) - hash) + serviceName.charCodeAt(i);
    hash = hash & hash;
  }
  return SERVICE_COLORS[Math.abs(hash) % SERVICE_COLORS.length];
};

// Custom node component - Modern card design
function StepNode({ data }: { data: any }) {
  const getStatusStyle = (status: string) => {
    switch (status) {
      case 'success':
        return { 
          border: '#22c55e',
          iconBg: '#dcfce7', 
          iconColor: '#16a34a',
          badge: 'bg-green-100 text-green-700'
        };
      case 'failed':
        return { 
          border: '#ef4444',
          iconBg: '#fee2e2', 
          iconColor: '#dc2626',
          badge: 'bg-red-100 text-red-700'
        };
      case 'in_progress':
        return { 
          border: '#3b82f6',
          iconBg: '#dbeafe', 
          iconColor: '#2563eb',
          badge: 'bg-blue-100 text-blue-700'
        };
      default:
        return { 
          border: '#d1d5db',
          iconBg: '#f3f4f6', 
          iconColor: '#6b7280',
          badge: 'bg-gray-100 text-gray-700'
        };
    }
  };

  const getIcon = (status: string) => {
    switch (status) {
      case 'success':
        return <CheckCircle2 className="w-4 h-4" />;
      case 'failed':
        return <XCircle className="w-4 h-4" />;
      case 'in_progress':
        return <Clock className="w-4 h-4 animate-pulse" />;
      default:
        return <Clock className="w-4 h-4" />;
    }
  };

  const statusStyle = getStatusStyle(data.status);
  const serviceColor = getServiceColor(data.actorService || '');
  const step = data.step;
  const hasValidations = data.validationCount > 0;
  const hasCritical = data.hasCriticalValidation;

  // Format step name for display (convert snake_case to Title Case)
  const formatStepName = (name: string) => {
    return name.split('_').map(word => 
      word.charAt(0).toUpperCase() + word.slice(1)
    ).join(' ');
  };

  return (
    <div
      className="bg-white rounded-xl shadow-sm min-w-[200px] max-w-[240px] cursor-pointer hover:shadow-lg transition-all border-l-4 relative"
      style={{ borderLeftColor: statusStyle.border }}
    >
      {/* Handles for edge connections (hidden) */}
      <Handle type="target" position={Position.Left} style={{ visibility: 'hidden' }} />
      <Handle type="source" position={Position.Right} style={{ visibility: 'hidden' }} />

      {/* Service Badge - Only show if NOT in a group */}
      {data.actorService && !data.inGroup && (
        <div 
          className="absolute -top-2.5 left-3 px-2 py-0.5 text-[10px] font-medium rounded-full border bg-white shadow-sm"
          style={{ borderColor: serviceColor.border, color: serviceColor.text }}
        >
          {data.actorService}
        </div>
      )}

      {/* Main Content */}
      <div className="px-4 pt-4 pb-3">
        {/* Title Row */}
        <div className="flex items-start gap-2 mb-1">
          <div 
            className="w-7 h-7 rounded-lg flex items-center justify-center flex-shrink-0"
            style={{ backgroundColor: statusStyle.iconBg, color: statusStyle.iconColor }}
          >
            {getIcon(data.status)}
          </div>
          <div className="flex-1 min-w-0">
            <div 
              className="font-semibold text-gray-900 text-sm leading-tight truncate hover:text-blue-600 cursor-copy transition-colors group relative"
              onClick={(e) => {
                e.stopPropagation();
                navigator.clipboard.writeText(data.label);
                // Optional: Show brief feedback (could use a toast or tooltip system)
              }}
              title="Click to copy step name"
            >
              {formatStepName(data.label)}
            </div>
            {step.idempotencyKey && (
              <div className="flex items-center gap-1.5 mt-0.5 group/idemp">
                <div className="text-[10px] text-gray-400 font-mono truncate">
                  {step.idempotencyKey}
                </div>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    navigator.clipboard.writeText(`${data.label}:${step.idempotencyKey}`);
                  }}
                  className="opacity-0 group-hover/idemp:opacity-100 transition-opacity p-0.5 hover:bg-gray-100 rounded text-gray-400 hover:text-gray-600"
                  title="Copy step:idempotency"
                >
                  <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                  </svg>
                </button>
              </div>
            )}
          </div>
        </div>

        {/* Actor/Description */}
        {data.actor && (
          <div className="text-xs text-gray-500 mt-2 truncate">
            {data.actor}
          </div>
        )}
      </div>

      {/* Footer Stats */}
      <div className="px-4 py-2 border-t border-gray-100 flex items-center gap-3 text-[10px]">
        {/* Time */}
        {step.lastUpdatedAt && (
          <div className="flex items-center gap-1 text-gray-400">
            <Clock className="w-3 h-3" />
            <span>{new Date(step.lastUpdatedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
          </div>
        )}

        {/* Retries */}
        {data.retryCount > 0 && (
          <div className="flex items-center gap-1 text-orange-500">
            <RefreshCw className="w-3 h-3" />
            <span>{data.retryCount}</span>
          </div>
        )}

        {/* Hash Chain Verification Indicator */}
        {data.hashChainValid !== undefined && (
          <div 
            className={`flex items-center gap-0.5 group relative cursor-help ${data.hashChainValid ? 'text-green-500' : 'text-red-500'}`}
            title={data.hashChainValid ? 'Hash chain verified - prevHash matches previous step' : 'Hash chain broken - prevHash does not match previous step'}
          >
            {data.hashChainValid ? <Link2 className="w-3 h-3" /> : <Link2Off className="w-3 h-3" />}
            {/* Tooltip */}
            <div className="absolute bottom-full left-1/2 transform -translate-x-1/2 mb-2 px-2 py-1 bg-gray-900 text-white text-[10px] rounded whitespace-nowrap opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-50">
              {data.hashChainValid ? 'Hash chain verified' : 'Hash chain broken'}
              <div className="absolute top-full left-1/2 transform -translate-x-1/2 border-4 border-transparent border-t-gray-900"></div>
            </div>
          </div>
        )}

        {/* Validation Issues */}
        {hasValidations && (
          <div className={`flex items-center gap-1 ${
            hasCritical ? 'text-red-600' : 'text-yellow-600'
          }`}>
            <AlertTriangle className="w-3 h-3" />
            <span>{data.validationCount}</span>
          </div>
        )}

        {/* Status Badge */}
        <div className={`ml-auto px-1.5 py-0.5 rounded text-[9px] font-medium ${statusStyle.badge}`}>
          {data.status === 'in_progress' ? 'Running' : data.status}
        </div>
      </div>
    </div>
  );
}

// Service group node - container with service label
function ServiceGroupNode({ data }: { data: any }) {
  const serviceColor = data.serviceColor || getServiceColor(data.label);
  
  return (
    <div className="w-full h-full relative">
      {/* Service Label */}
      <div 
        className="absolute -top-3 left-4 px-3 py-1 text-xs font-semibold rounded-full border-2 bg-white shadow-sm"
        style={{ borderColor: serviceColor.border, color: serviceColor.text }}
      >
        {data.label}
      </div>
    </div>
  );
}

const nodeTypes = {
  stepNode: StepNode,
  group: ServiceGroupNode,
};

export default function ThreadGraphView({ steps, validations = [], onNodeClick }: ThreadGraphViewProps) {
  // Get thread ID from first step
  const threadId = steps.length > 0 ? steps[0].threadId : null;

  // Fetch thread integrity verification
  const { data: threadIntegrity, isLoading: integrityLoading } = useQuery({
    queryKey: ['threadIntegrity', threadId],
    queryFn: async () => {
      if (!threadId) return null;
      try {
        return await graphqlClient.verifyThreadIntegrity(threadId);
      } catch (error) {
        console.error('Failed to verify thread integrity:', error);
        return null;
      }
    },
    enabled: threadId !== null,
  });

  // Fetch step histories to get actor info
  const { data: stepHistories } = useQuery({
    queryKey: ['stepHistoriesForGraph', steps.map(s => `${s.stepName}:${s.idempotencyKey}`)],
    queryFn: async () => {
      if (steps.length === 0) return [];
      const historyPromises = steps.map(step => 
        graphqlClient.getStepHistory(step.threadId, step.stepName, step.idempotencyKey, 1)
      );
      const results = await Promise.all(historyPromises);
      return results.map((history, idx) => ({
        step: steps[idx],
        history: history[0] || null,
      }));
    },
    enabled: steps.length > 0,
  });

  // Extract unique actor IDs from step histories
  const actorIds = new Set<string>();
  stepHistories?.forEach(item => {
    if (item.history?.actor) {
      actorIds.add(item.history.actor);
    }
  });

  // Resolve actor IDs to names
  const { data: resolvedActors } = useQuery({
    queryKey: ['resolveActors', Array.from(actorIds)],
    queryFn: () => graphqlClient.resolveActors(Array.from(actorIds)),
    enabled: actorIds.size > 0,
  });

  // Create a map of actor ID to name
  const actorMap = new Map<string, string>();
  resolvedActors?.forEach(actor => {
    actorMap.set(actor.id, actor.name);
  });

  // Sort steps using hash chain (cryptographic ordering)
  const stepsWithHistory = stepHistories || steps.map(s => ({ step: s, history: null }));
  
  const sortStepsByHashChain = (stepsData: Array<{ step: any; history: any }>) => {
    if (!stepsData || stepsData.length === 0) return [];
    
    // Build hash map for quick lookup
    const hashMap = new Map<string, { step: any; history: any }>();
    stepsData.forEach(item => {
      if (item.step.hash) {
        hashMap.set(item.step.hash, item);
      }
    });
    
    // Find genesis (step with no prevHash)
    const genesis = stepsData.find(item => !item.step.prevHash);
    if (!genesis) {
      // Fallback to timestamp sorting if no genesis found
      return [...stepsData].sort((a, b) => {
        const timeA = new Date(a.step.firstSeenAt).getTime();
        const timeB = new Date(b.step.firstSeenAt).getTime();
        return timeA - timeB;
      });
    }
    
    // Build ordered list by following the hash chain
    const ordered: Array<{ step: any; history: any }> = [genesis];
    const orderedHashes = new Set<string>([genesis.step.hash]);
    let current = genesis;
    
    while (ordered.length < stepsData.length) {
      // Find next step (step whose prevHash matches current step's hash)
      const next = stepsData.find(item => 
        item.step.prevHash === current.step.hash && 
        !orderedHashes.has(item.step.hash)
      );
      
      if (!next) break; // Chain ends or is broken
      
      ordered.push(next);
      orderedHashes.add(next.step.hash);
      current = next;
    }
    
    // Add any remaining steps that aren't in the chain (shouldn't happen with valid chain)
    stepsData.forEach(item => {
      if (!ordered.includes(item)) {
        ordered.push(item);
      }
    });
    
    return ordered;
  };
  
  const sortedStepsWithHistory = sortStepsByHashChain(stepsWithHistory);

  // Group ALL steps by actorService (not consecutive, but all steps from same service)
  type ServiceGroup = {
    service: string;
    steps: Array<{ step: any; history: any; index: number }>;
  };

  const serviceMap = new Map<string, Array<{ step: any; history: any; index: number }>>();
  
  sortedStepsWithHistory.forEach((item, index) => {
    const service = item.history?.actorService || 'Unknown Service';
    
    if (!serviceMap.has(service)) {
      serviceMap.set(service, []);
    }
    
    serviceMap.get(service)!.push({ step: item.step, history: item.history, index });
  });

  // Convert map to array of groups, maintaining the order of first appearance
  const serviceGroups: ServiceGroup[] = [];
  const seenServices = new Set<string>();
  
  sortedStepsWithHistory.forEach((item) => {
    const service = item.history?.actorService || 'Unknown Service';
    if (!seenServices.has(service)) {
      seenServices.add(service);
      serviceGroups.push({
        service,
        steps: serviceMap.get(service)!,
      });
    }
  });

  // Build hash→step map for edge creation using hash chain
  const hashToStepMap = new Map<string, any>();
  const prevHashToStepMap = new Map<string, any>();
  const allSteps = sortedStepsWithHistory.map(item => item.step);
  
  allSteps.forEach(step => {
    if (step.hash) {
      hashToStepMap.set(step.hash, step);
    }
    if (step.prevHash) {
      prevHashToStepMap.set(step.prevHash, step);
    }
  });

  // Build nodes with service groups
  const initialNodes: Node[] = [];
  const stepNodeMap = new Map<string, Node>();
  
  const STEP_WIDTH = 220;
  const STEP_HEIGHT = 140;
  const STEP_GAP = 40;
  const GROUP_PADDING = 20;
  const GROUP_GAP = 60;
  const START_Y = 100;

  // Calculate absolute positions for all steps first
  let currentX = 50;
  const stepPositions = new Map<number, { x: number; y: number }>();
  
  sortedStepsWithHistory.forEach((item, globalIndex) => {
    stepPositions.set(globalIndex, { x: currentX, y: START_Y });
    currentX += STEP_WIDTH + STEP_GAP;
  });

  // Create groups and nodes
  let groupStartIndex = 0;
  serviceGroups.forEach((group, groupIndex) => {
    const serviceColor = getServiceColor(group.service);
    const groupStepCount = group.steps.length;
    
    // Calculate group bounds from step positions
    const firstStepPos = stepPositions.get(groupStartIndex)!;
    const lastStepPos = stepPositions.get(groupStartIndex + groupStepCount - 1)!;
    
    const groupX = firstStepPos.x - GROUP_PADDING;
    const groupWidth = (lastStepPos.x - firstStepPos.x) + STEP_WIDTH + (GROUP_PADDING * 2);
    const groupHeight = STEP_HEIGHT + GROUP_PADDING * 2 + 30;
    
    // Create group background
    initialNodes.push({
      id: `group-${groupIndex}`,
      type: 'group',
      position: { x: groupX, y: START_Y - GROUP_PADDING - 30 },
      style: {
        width: groupWidth,
        height: groupHeight,
        backgroundColor: serviceColor.bg,
        borderRadius: 12,
        border: `2px solid ${serviceColor.border}`,
        padding: 0,
        zIndex: 0,
      },
      data: { 
        label: group.service,
        serviceColor,
      },
      selectable: false,
      draggable: false,
    });

    // Create step nodes with absolute positioning (NOT relative to parent)
    group.steps.forEach((item, stepIndex) => {
      const step = item.step;
      const history = item.history;
      const globalIndex = item.index;
      
      // Count validations
      const stepValidations = validations.filter(
        v => v.stepName === step.stepName && v.idempotencyKey === step.idempotencyKey
      );
      const validationsWithIssues = stepValidations.filter(v => 
        v.hasCriticalViolation || 
        v.criticalCount > 0 || 
        v.warningCount > 0 || 
        v.validations.length > 0
      );
      const hasCritical = stepValidations.some(v => v.hasCriticalViolation);

      // Check hash chain validity
      let hashChainValid: boolean | undefined = undefined;
      if (globalIndex === 0) {
        hashChainValid = !step.prevHash || step.prevHash === '';
      } else {
        const prevStepInOrder = allSteps[globalIndex - 1];
        if (prevStepInOrder && step.prevHash) {
          hashChainValid = step.prevHash === prevStepInOrder.hash;
        }
      }

      const nodeId = `step-${globalIndex}`;
      const pos = stepPositions.get(globalIndex)!;

      const stepNode: Node = {
        id: nodeId,
        type: 'stepNode',
        position: { x: pos.x, y: pos.y }, // Absolute position
        zIndex: 10, // Above groups
        data: {
          label: step.stepName,
          status: step.status,
          retryCount: step.retryCount,
          step: step,
          actor: history?.actor ? (actorMap.get(history.actor) || history.actor) : '',
          actorService: history?.actorService || '',
          validationCount: validationsWithIssues.length,
          hasCriticalValidation: hasCritical,
          inGroup: true, // Hide service badge since group shows it
          hash: step.hash,
          prevHash: step.prevHash,
          hashChainValid,
          globalIndex,
        },
      };
      
      initialNodes.push(stepNode);
      stepNodeMap.set(nodeId, stepNode);
    });

    groupStartIndex += groupStepCount;
  });

  // Build edges sequentially (step-0 → step-1 → step-2 → ...)
  const initialEdges: Edge[] = [];
  
  for (let i = 1; i < allSteps.length; i++) {
    const prevStep = allSteps[i - 1];
    const currStep = allSteps[i];
    
    const sourceNodeId = `step-${i - 1}`;
    const targetNodeId = `step-${i}`;
    
    // Color edge based on source step status
    let edgeColor = '#d1d5db';
    if (prevStep.status === 'success') edgeColor = '#86efac';
    else if (prevStep.status === 'failed') edgeColor = '#fca5a5';
    else if (prevStep.status === 'in_progress') edgeColor = '#93c5fd';
    
    initialEdges.push({
      id: `edge-${i - 1}-${i}`,
      source: sourceNodeId,
      target: targetNodeId,
      type: 'smoothstep',
      animated: currStep.status === 'in_progress',
      zIndex: 1000,
      markerEnd: {
        type: MarkerType.ArrowClosed,
        width: 12,
        height: 12,
        color: edgeColor,
      },
      style: { 
        strokeWidth: 2, 
        stroke: edgeColor,
      },
    });
  }

  // Debug: Log edge creation stats
  console.log('[ReactFlow] Nodes created:', initialNodes.length, 'Edges created:', initialEdges.length);
  
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Update nodes when step histories or actor resolution changes
  useEffect(() => {
    if (stepHistories && stepHistories.length > 0) {
      setNodes(initialNodes);
      setEdges(initialEdges);
    }
  }, [stepHistories, resolvedActors, setNodes, setEdges]);

  const onNodeClickHandler = useCallback(
    (_: React.MouseEvent, node: Node) => {
      if (onNodeClick && node.data.step) {
        onNodeClick(node.data.step);
      }
    },
    [onNodeClick]
  );

  if (steps.length === 0) {
    return (
      <div className="flex items-center justify-center h-96 text-gray-500 border-2 border-gray-200 rounded-lg bg-gray-50">
        <div className="text-center">
          <Clock className="w-12 h-12 mx-auto mb-3 text-gray-400" />
          <p>No steps to visualize</p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      {/* Hash Verification Status Bar */}
      {integrityLoading ? (
        <div className="px-4 py-3 bg-blue-50 border border-blue-200 rounded-lg flex items-center gap-2 text-blue-700 text-sm">
          <Clock className="w-4 h-4 animate-spin" />
          <span>Verifying hash chain integrity...</span>
        </div>
      ) : threadIntegrity ? (
        <div className={`px-4 py-3 rounded-lg flex items-center gap-3 text-sm border ${
          threadIntegrity.verified 
            ? 'bg-green-50 border-green-200 text-green-700' 
            : 'bg-red-50 border-red-200 text-red-700'
        }`}>
          {threadIntegrity.verified ? (
            <>
              <Shield className="w-4 h-4 flex-shrink-0" />
              <div className="flex-1">
                <span className="font-medium">Hash chain verified</span>
                <span className="text-xs opacity-75 ml-2">
                  {threadIntegrity.validSteps} of {threadIntegrity.totalSteps} steps valid
                </span>
              </div>
            </>
          ) : (
            <>
              <AlertCircle className="w-4 h-4 flex-shrink-0" />
              <div className="flex-1">
                <span className="font-medium">Hash chain integrity issue</span>
                <span className="text-xs opacity-75 ml-2">
                  {threadIntegrity.brokenLinks} broken link{threadIntegrity.brokenLinks !== 1 ? 's' : ''}
                </span>
              </div>
            </>
          )}
        </div>
      ) : null}

      {/* React Flow Container */}
      <div className="border-2 border-gray-200 rounded-lg bg-gray-50 flex-1" style={{ height: 'calc(100vh - 380px)' }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={onNodeClickHandler}
        nodeTypes={nodeTypes}
        fitView
        attributionPosition="bottom-left"
      >
        <Background />
        <Controls />
        <MiniMap
          nodeColor={(node) => {
            const status = node.data.status;
            switch (status) {
              case 'success':
                return '#22c55e';
              case 'failed':
                return '#ef4444';
              case 'in_progress':
                return '#3b82f6';
              default:
                return '#9ca3af';
            }
          }}
        />
      </ReactFlow>
      </div>
    </div>
  );
}

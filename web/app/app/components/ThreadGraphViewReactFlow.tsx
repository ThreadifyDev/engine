import { useCallback, useEffect, useState, useMemo } from 'react';
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
  useReactFlow,
  ReactFlowProvider,
} from 'reactflow';
import 'reactflow/dist/style.css';
import { type StepStateInfo, graphqlClient } from '~/lib/graphql';
import { CheckCircle2, XCircle, Clock, RefreshCw, AlertTriangle, Search, X, ChevronLeft, ChevronRight } from 'lucide-react';

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

      {/* Retry Badge - Only show if retryCount > 1 */}
      {data.retryCount > 1 && (
        <div className="absolute -top-2 -right-2 bg-blue-500 text-white rounded-full px-2 py-0.5 text-xs font-bold shadow-md flex items-center gap-1">
          <RefreshCw className="w-3 h-3" />
          {data.retryCount}
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

        {/* Retries - Only show if more than 1 retry */}
        {data.retryCount > 1 && (
          <div className="flex items-center gap-1 text-blue-600 font-semibold">
            <RefreshCw className="w-3 h-3" />
            <span>{data.retryCount}</span>
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

function ThreadGraphViewInner({ steps, validations = [], onNodeClick }: ThreadGraphViewProps) {
  // Sort steps by timestamp (firstSeenAt) - memoized to prevent recreation
  const sortedSteps = useMemo(() => {
    return [...steps].sort((a, b) => {
      const timeA = new Date(a.firstSeenAt).getTime();
      const timeB = new Date(b.firstSeenAt).getTime();
      return timeA - timeB;
    });
  }, [steps]);

  // Extract unique actor IDs and resolve them
  const actorIds = Array.from(new Set(sortedSteps.map(s => s.actor).filter(Boolean)));
  const { data: resolvedActors } = useQuery({
    queryKey: ['resolveActors', actorIds],
    queryFn: () => graphqlClient.resolveActors(actorIds as string[]),
    enabled: actorIds.length > 0,
  });

  // Create actor ID to name map
  const actorMap = useMemo(() => {
    const map = new Map<string, string>();
    resolvedActors?.forEach(actor => {
      map.set(actor.id, actor.name);
    });
    return map;
  }, [resolvedActors]);

  // Memoize nodes and edges to prevent hydration errors
  const { initialNodes, initialEdges } = useMemo(() => {
  // Group steps by actorService
  type ServiceGroup = {
    service: string;
    steps: Array<{ step: StepStateInfo; index: number }>;
  };

  const serviceMap = new Map<string, Array<{ step: StepStateInfo; index: number }>>();
  
  sortedSteps.forEach((step, index) => {
    const service = step.actorService || 'Unknown Service';
    
    if (!serviceMap.has(service)) {
      serviceMap.set(service, []);
    }
    
    serviceMap.get(service)!.push({ step, index });
  });

  // Convert map to array of groups, maintaining the order of first appearance
  const serviceGroups: ServiceGroup[] = [];
  const seenServices = new Set<string>();
  
  sortedSteps.forEach((step) => {
    const service = step.actorService || 'Unknown Service';
    if (!seenServices.has(service)) {
      seenServices.add(service);
      serviceGroups.push({
        service,
        steps: serviceMap.get(service)!,
      });
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

  // Calculate absolute positions for all steps with dynamic Y positioning
  let currentX = 50;
  const stepPositions = new Map<number, { x: number; y: number }>();
  
  sortedSteps.forEach((step, globalIndex) => {
    // Add some vertical variation for visual interest
    // Alternate between slightly higher and lower positions
    const yVariation = (globalIndex % 3 === 0) ? -20 : (globalIndex % 3 === 1) ? 20 : 0;
    const dynamicY = START_Y + yVariation;
    
    stepPositions.set(globalIndex, { x: currentX, y: dynamicY });
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
    group.steps.forEach((item) => {
      const step = item.step;
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
          actor: step.actor ? (actorMap.get(step.actor) || step.actor) : '',
          actorService: step.actorService || '',
          validationCount: validationsWithIssues.length,
          hasCriticalValidation: hasCritical,
          inGroup: true, // Hide service badge since group shows it
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
  
  for (let i = 1; i < sortedSteps.length; i++) {
    const prevStep = sortedSteps[i - 1];
    const currStep = sortedSteps[i];
    
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
  
  return { initialNodes, initialEdges };
  }, [sortedSteps, validations, actorMap]);
  
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);
  const [searchQuery, setSearchQuery] = useState('');
  const [matchedNodes, setMatchedNodes] = useState<Node[]>([]);
  const [currentMatchIndex, setCurrentMatchIndex] = useState(0);
  const { fitView, setCenter } = useReactFlow();

  // Update nodes when steps or resolved actors change
  useEffect(() => {
    if (steps.length > 0) {
      setNodes(initialNodes);
      setEdges(initialEdges);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [steps, resolvedActors]);

  // Search functionality
  const handleSearch = useCallback((query: string) => {
    setSearchQuery(query);
    
    if (!query.trim()) {
      // Reset all nodes to normal state
      setNodes((nds) =>
        nds.map((node) => ({
          ...node,
          style: {
            ...node.style,
            opacity: 1,
          },
        }))
      );
      setMatchedNodes([]);
      setCurrentMatchIndex(0);
      return;
    }

    const lowerQuery = query.toLowerCase();

    // Find all matching nodes
    setNodes((nds) => {
      const matches: Node[] = [];
      
      const updatedNodes = nds.map((node) => {
        const isMatch =
          node.data.label?.toLowerCase().includes(lowerQuery) ||
          node.data.actor?.toLowerCase().includes(lowerQuery) ||
          node.data.actorService?.toLowerCase().includes(lowerQuery) ||
          node.data.status?.toLowerCase().includes(lowerQuery);

        // Collect all matching step nodes
        if (isMatch && node.type === 'stepNode') {
          matches.push(node);
        }

        return {
          ...node,
          style: {
            ...node.style,
            opacity: isMatch || node.type === 'group' ? 1 : 0.3,
          },
        };
      });

      // Store matched nodes and reset to first match
      setMatchedNodes(matches);
      setCurrentMatchIndex(0);

      // Center on first match
      if (matches.length > 0) {
        const firstMatch = matches[0];
        const nodeX = firstMatch.position.x;
        const nodeY = firstMatch.position.y;
        
        requestAnimationFrame(() => {
          setCenter(nodeX + 110, nodeY + 70, {
            zoom: 1.2,
            duration: 800,
          });
        });
      }

      return updatedNodes;
    });
  }, [setNodes, setCenter]);

  // Navigate to next match
  const handleNextMatch = useCallback(() => {
    if (matchedNodes.length === 0) return;
    
    const nextIndex = (currentMatchIndex + 1) % matchedNodes.length;
    setCurrentMatchIndex(nextIndex);
    
    const node = matchedNodes[nextIndex];
    setCenter(node.position.x + 110, node.position.y + 70, {
      zoom: 1.2,
      duration: 800,
    });
  }, [matchedNodes, currentMatchIndex, setCenter]);

  // Navigate to previous match
  const handlePrevMatch = useCallback(() => {
    if (matchedNodes.length === 0) return;
    
    const prevIndex = (currentMatchIndex - 1 + matchedNodes.length) % matchedNodes.length;
    setCurrentMatchIndex(prevIndex);
    
    const node = matchedNodes[prevIndex];
    setCenter(node.position.x + 110, node.position.y + 70, {
      zoom: 1.2,
      duration: 800,
    });
  }, [matchedNodes, currentMatchIndex, setCenter]);

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
    <div className="border-2 border-gray-200 rounded-lg bg-gray-50 relative" style={{ height: 'calc(100vh - 280px)' }}>
      {/* Search Bar */}
      <div className="absolute top-4 left-4 z-10 flex items-center gap-2">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-gray-400" />
          <input
            type="text"
            placeholder="Search steps, services, status..."
            value={searchQuery}
            onChange={(e) => handleSearch(e.target.value)}
            className="pl-10 pr-10 py-2 w-80 border border-gray-300 rounded-lg bg-white text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
          />
          {searchQuery && (
            <button
              onClick={() => handleSearch('')}
              className="absolute right-3 top-1/2 transform -translate-y-1/2 text-gray-400 hover:text-gray-600"
            >
              <X className="w-4 h-4" />
            </button>
          )}
        </div>
        
        {/* Navigation buttons - only show when there are matches */}
        {matchedNodes.length > 1 && (
          <div className="flex items-center gap-1 bg-white border border-gray-300 rounded-lg px-2 py-1">
            <button
              onClick={handlePrevMatch}
              className="p-1 hover:bg-gray-100 rounded transition-colors"
              title="Previous match"
            >
              <ChevronLeft className="w-4 h-4 text-gray-600" />
            </button>
            <span className="text-xs text-gray-600 px-2 font-medium">
              {currentMatchIndex + 1} / {matchedNodes.length}
            </span>
            <button
              onClick={handleNextMatch}
              className="p-1 hover:bg-gray-100 rounded transition-colors"
              title="Next match"
            >
              <ChevronRight className="w-4 h-4 text-gray-600" />
            </button>
          </div>
        )}
      </div>

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
  );
}

// Wrapper component with ReactFlowProvider
export default function ThreadGraphView(props: ThreadGraphViewProps) {
  return (
    <ReactFlowProvider>
      <ThreadGraphViewInner {...props} />
    </ReactFlowProvider>
  );
}

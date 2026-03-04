import { useEffect, useMemo, useState } from "react";
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
  Position,
  Handle,
} from 'reactflow';
import 'reactflow/dist/style.css';
import { Clock, CheckCircle2, AlertCircle, Search, X, ChevronRight, AlertTriangle, XCircle, RefreshCw, Loader2, Star } from 'lucide-react';
import { type StepStateInfo, graphqlClient } from '~/lib/graphql';

interface ThreadGraphViewProps {
  steps: StepStateInfo[];
  contractName: string;
  contractVersion?: number;
  onNodeClick?: (step: StepStateInfo) => void;
}

// Color palette for different parties (same as ContractGraphView)
const PARTY_COLORS: Record<string, { bg: string; border: string; text: string }> = {
  merchant: { bg: 'bg-blue-50', border: 'border-blue-300', text: 'text-blue-900' },
  payment_processor: { bg: 'bg-green-50', border: 'border-green-300', text: 'text-green-900' },
  warehouse_manager: { bg: 'bg-purple-50', border: 'border-purple-300', text: 'text-purple-900' },
  logistics_carrier: { bg: 'bg-orange-50', border: 'border-orange-300', text: 'text-orange-900' },
  supplier: { bg: 'bg-pink-50', border: 'border-pink-300', text: 'text-pink-900' },
  customer: { bg: 'bg-yellow-50', border: 'border-yellow-300', text: 'text-yellow-900' },
};

// Default color for unknown parties
const DEFAULT_COLOR = { bg: 'bg-gray-50', border: 'border-gray-300', text: 'text-gray-900' };

// Custom node component - Shows contract step with execution overlay
function ContractStepNode({ data }: { data: any }) {
  const colors = PARTY_COLORS[data.owner] || DEFAULT_COLOR;
  const isEntryPoint = data.isEntryPoint;
  const isTerminal = data.isTerminal;
  
  // Execution status from actual thread steps
  const executionStatus = data.executionStatus; // 'success', 'failed', 'violated', 'in_progress', 'not_executed'
  const hasExecuted = executionStatus && executionStatus !== 'not_executed';
  const retryCount = data.retryCount || 0;

  // Status styling
  const getStatusBorder = () => {
    if (!hasExecuted) return colors.border;
    switch (executionStatus) {
      case 'success': return 'border-green-500';
      case 'failed': return 'border-red-500';
      case 'violated': return 'border-orange-500';
      case 'in_progress': return 'border-blue-500';
      default: return colors.border;
    }
  };

  const getStatusIcon = () => {
    if (!hasExecuted) return null;
    switch (executionStatus) {
      case 'success': return <CheckCircle2 className="w-4 h-4 text-green-600" />;
      case 'failed': return <XCircle className="w-4 h-4 text-red-600" />;
      case 'violated': return <AlertTriangle className="w-4 h-4 text-orange-600" />;
      case 'in_progress': return <Clock className="w-4 h-4 text-blue-600 animate-pulse" />;
      default: return null;
    }
  };

  return (
    <>
      {/* Handles for edge connections */}
      <Handle type="target" position={Position.Top} style={{ visibility: 'hidden' }} />
      <Handle type="source" position={Position.Bottom} style={{ visibility: 'hidden' }} />
      
      <div className={`rounded-xl shadow-md min-w-[200px] max-w-[240px] border-2 relative ${colors.bg} ${getStatusBorder()}`}>
        
        {/* Party Badge (top) */}
        <div 
          className={`absolute -top-2.5 left-3 px-2 py-0.5 text-[10px] font-medium rounded-full border shadow-sm ${colors.bg} ${colors.border} ${colors.text}`}
        >
          {data.owner}
        </div>

        {/* Terminal/Entry Point Badge (top right) */}
        {(isTerminal || isEntryPoint) && (
          <div className="absolute -top-2 -right-2 bg-blue-500 text-white rounded-full px-2 py-0.5 text-xs font-bold shadow-md flex items-center gap-1">
            {isTerminal && <CheckCircle2 className="w-3 h-3" />}
            {isEntryPoint && <Star className="w-3 h-3 fill-white" />}
          </div>
        )}

        {/* Retry Badge - Only if executed with retries */}
        {hasExecuted && retryCount > 1 && (
          <div className="absolute -top-2 -right-2 bg-blue-500 text-white rounded-full px-2 py-0.5 text-xs font-bold shadow-md flex items-center gap-1">
            <RefreshCw className="w-3 h-3" />
            {retryCount}
          </div>
        )}

        {/* Main Content */}
        <div className="px-4 pt-4 pb-1">
          {/* Step Name */}
          <div className="flex items-center gap-2 mb-2">
            <div className="text-base font-semibold text-gray-900 flex-1">
              {data.label}
            </div>
            {/* Execution Status Icon */}
            {getStatusIcon()}
          </div>

          {/* Metadata */}
          <div className="space-y-1">
            {/* Timeout */}
            {data.timeout && (
              <div className="flex items-center gap-1.5 text-xs text-gray-600">
                <Clock className="w-3.5 h-3.5" />
                <span>{data.timeout}</span>
              </div>
            )}

            {/* Required indicator */}
            {data.required && (
              <div className="flex items-center gap-1.5 text-xs text-red-600">
                <AlertCircle className="w-3.5 h-3.5" />
                <span>Required</span>
              </div>
            )}

            {/* Execution Status Badge */}
            {hasExecuted && (
              <div className="mt-2 pt-2 border-t border-gray-200">
                <div className={`inline-flex px-2 py-0.5 text-xs font-medium rounded-full ${
                  executionStatus === 'success' ? 'bg-green-100 text-green-800' :
                  executionStatus === 'failed' ? 'bg-red-100 text-red-800' :
                  executionStatus === 'violated' ? 'bg-orange-100 text-orange-800' :
                  executionStatus === 'in_progress' ? 'bg-blue-100 text-blue-800' :
                  'bg-gray-100 text-gray-800'
                }`}>
                  {executionStatus === 'in_progress' ? 'Running' : executionStatus}
                </div>
              </div>
            )}

            {/* Not Executed Badge */}
            {!hasExecuted && (
              <div className="mt-2 pt-2 border-t border-gray-200">
                <div className="inline-flex px-2 py-0.5 text-xs font-medium rounded-full bg-gray-100 text-gray-500">
                  Not executed
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </>
  );
}

const nodeTypes = {
  contractStepNode: ContractStepNode,
};

// Custom hierarchical layout using BFS traversal (same as ContractGraphView)
function getLayoutedElements(
  nodes: Node[],
  edges: Edge[],
  graphData: any
): { nodes: Node[]; edges: Edge[] } {
  const nodeWidth = 240;
  const nodeHeight = 120;
  const horizontalGap = 100;
  const verticalGap = 150;

  // Access the nested graph structure
  const graph = graphData.graph?.graph || graphData.graph;
  
  // Build adjacency map from node.next
  const nodeMap = new Map<string, any>();
  Object.values(graph.nodes).forEach((node: any) => {
    nodeMap.set(node.id, node);
  });

  // BFS to assign levels
  const levels: string[][] = [];
  const visited = new Set<string>();
  const nodeLevel = new Map<string, number>();

  // Start from entry points
  const queue: Array<{ id: string; level: number }> = [];
  (graph.entryPoints || graph.entry_points || []).forEach((entryId: string) => {
    queue.push({ id: entryId, level: 0 });
  });

  while (queue.length > 0) {
    const { id, level } = queue.shift()!;
    
    if (visited.has(id)) continue;
    visited.add(id);
    nodeLevel.set(id, level);

    if (!levels[level]) levels[level] = [];
    levels[level].push(id);

    const node = nodeMap.get(id);
    if (node?.next) {
      node.next.forEach((nextId: string) => {
        if (!visited.has(nextId)) {
          queue.push({ id: nextId, level: level + 1 });
        }
      });
    }
  }

  // Position nodes
  const layoutedNodes = nodes.map((node) => {
    const level = nodeLevel.get(node.id) || 0;
    const nodesInLevel = levels[level] || [];
    const indexInLevel = nodesInLevel.indexOf(node.id);
    
    const totalWidth = nodesInLevel.length * nodeWidth + (nodesInLevel.length - 1) * horizontalGap;
    const startX = -totalWidth / 2;

    return {
      ...node,
      position: {
        x: startX + indexInLevel * (nodeWidth + horizontalGap),
        y: level * (nodeHeight + verticalGap),
      },
    };
  });

  return { nodes: layoutedNodes, edges };
}

export default function ThreadGraphView({ steps, contractName, contractVersion, onNodeClick }: ThreadGraphViewProps) {
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [showSearch, setShowSearch] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  // Fetch contract graph
  const { data: contractGraph, isLoading } = useQuery({
    queryKey: ['contractGraph', contractName, contractVersion],
    queryFn: () => graphqlClient.getContractGraph(contractName, contractVersion),
    enabled: !!contractName,
  });

  const graphData = contractGraph;

  // Build execution map from steps
  const executionMap = useMemo(() => {
    const map = new Map<string, { status: string; retryCount: number; step: StepStateInfo }>();
    steps.forEach(step => {
      map.set(step.stepName, {
        status: step.status,
        retryCount: step.retryCount,
        step: step,
      });
    });
    return map;
  }, [steps]);

  // Build transition map for retry info
  const transitionMap = useMemo(() => {
    if (!graphData) return new Map();
    const map = new Map<string, { canRetry: boolean; maxRetries: number }>();
    graphData.transitions?.forEach((t: any) => {
      t.to.forEach((target: string) => {
        map.set(`${t.from}-${target}`, { canRetry: t.canRetry || false, maxRetries: t.maxRetries || 0 });
      });
    });
    return map;
  }, [graphData?.transitions]);

  useEffect(() => {
    if (!graphData?.graph?.nodes) return;

    // Convert nodes map to array
    const nodesArray = Object.values(graphData.graph.nodes);

    // Create nodes with execution overlay
    const initialNodes: Node[] = nodesArray.map((node: any) => {
      const execution = executionMap.get(node.id);
      
      return {
        id: node.id,
        type: 'contractStepNode',
        position: { x: 0, y: 0 }, // Will be set by layout
        data: {
          label: node.id,
          owner: node.owner || 'unknown',
          type: node.type,
          mode: node.mode,
          required: node.required,
          timeout: node.timeout,
          maxDuration: node.maxDuration,
          isEntryPoint: graphData.graph.entryPoints?.includes(node.id) || false,
          isTerminal: graphData.graph.terminalSteps?.includes(node.id) || false,
          // Execution overlay
          executionStatus: execution?.status || 'not_executed',
          retryCount: execution?.retryCount || 0,
          executedStep: execution?.step,
        },
      };
    });

    // Create edges from transitions array
    const initialEdges: Edge[] = [];
    graphData.transitions?.forEach((transition: any) => {
      transition.to?.forEach((target: string) => {
        const transitionKey = `${transition.from}-${target}`;
        const retryInfo = transitionMap.get(transitionKey);
        
        // Check if this transition was executed
        const sourceExecution = executionMap.get(transition.from);
        const targetExecution = executionMap.get(target);
        const wasExecuted = sourceExecution && targetExecution;

        // Color edge based on execution
        let edgeColor = '#94a3b8'; // Default gray
        let animated = false;
        
        if (wasExecuted) {
          if (sourceExecution.status === 'success') edgeColor = '#22c55e'; // green
          else if (sourceExecution.status === 'failed') edgeColor = '#ef4444'; // red
          else if (sourceExecution.status === 'violated') edgeColor = '#f97316'; // orange
          else if (sourceExecution.status === 'in_progress') {
            edgeColor = '#3b82f6'; // blue
            animated = true;
          }
        }

        initialEdges.push({
          id: transitionKey,
          source: transition.from,
          target: target,
          type: 'default',
          animated: animated,
          style: {
            stroke: edgeColor,
            strokeWidth: wasExecuted ? 3 : 2,
            strokeDasharray: retryInfo?.canRetry ? '5,5' : undefined,
            opacity: wasExecuted ? 1 : 0.4,
          },
          label: retryInfo?.maxRetries && retryInfo.maxRetries > 0 ? `retry: ${retryInfo.maxRetries}` : undefined,
          labelStyle: {
            fontSize: 11,
            fill: '#64748b',
            fontWeight: 600,
          },
          labelBgStyle: {
            fill: '#ffffff',
            fillOpacity: 0.95,
          },
          labelBgPadding: [8, 4] as [number, number],
          labelBgBorderRadius: 4,
          markerEnd: {
            type: MarkerType.ArrowClosed,
            color: edgeColor,
            width: 20,
            height: 20,
          },
        });
      });
    });

    // Apply custom hierarchical layout
    const { nodes: layoutedNodes, edges: layoutedEdges } = getLayoutedElements(
      initialNodes,
      initialEdges,
      graphData
    );

    setNodes(layoutedNodes);
    setEdges(layoutedEdges);
  }, [graphData, executionMap, transitionMap, setNodes, setEdges]);

  // Filter nodes based on search
  const filteredNodes = useMemo(() => {
    if (!searchTerm) return nodes;
    return nodes.map(node => ({
      ...node,
      style: {
        ...node.style,
        opacity: node.data.label.toLowerCase().includes(searchTerm.toLowerCase()) ? 1 : 0.2,
      }
    }));
  }, [nodes, searchTerm]);

  // Get selected node data
  const selectedNode = useMemo(() => {
    return nodes.find(n => n.id === selectedNodeId);
  }, [nodes, selectedNodeId]);

  // Show loading state
  if (isLoading) {
    return (
      <div className="w-full h-[calc(100vh-300px)] bg-white border border-gray-200 rounded flex items-center justify-center">
        <div className="text-center">
          <Loader2 className="w-12 h-12 mx-auto mb-3 text-gray-400 animate-spin" />
          <p className="text-gray-500">Loading contract graph...</p>
        </div>
      </div>
    );
  }

  // Show error state if no graph data
  if (!graphData) {
    return (
      <div className="w-full h-[calc(100vh-300px)] bg-white border border-gray-200 rounded flex items-center justify-center">
        <div className="text-center text-gray-500">
          <p className="text-lg font-medium mb-2">Failed to load contract graph</p>
          <p className="text-sm text-gray-400">Please try refreshing the page</p>
        </div>
      </div>
    );
  }

  return (
    <div className="w-full h-[calc(100vh-300px)] bg-white border border-gray-200 rounded relative">
      {/* Header with parties and controls */}
      <div className="absolute top-4 left-4 right-4 z-10 flex items-center justify-between gap-4">
        {/* Left: Parties legend */}
        <div className="bg-white border border-gray-200 rounded-lg px-4 py-2 shadow-sm max-w-md">
          <div className="flex items-center gap-4 overflow-x-auto">
            <span className="text-xs font-semibold text-gray-700 whitespace-nowrap">Parties:</span>
            <div className="flex items-center gap-3 overflow-x-auto pb-1">
              {graphData.parties?.map((party: string) => {
                const colors = PARTY_COLORS[party] || DEFAULT_COLOR;
                return (
                  <div key={party} className="flex items-center gap-1.5 whitespace-nowrap">
                    <div className={`w-2.5 h-2.5 rounded-full ${colors.bg} border ${colors.border}`} />
                    <span className="text-xs text-gray-600">{party}</span>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* Right: Controls */}
        <div className="flex items-center gap-2">
          {/* Search toggle */}
          {!showSearch ? (
            <button
              onClick={() => setShowSearch(true)}
              className="bg-white border border-gray-200 rounded-lg px-3 py-2 shadow-sm hover:bg-gray-50 transition-colors"
              title="Search steps"
            >
              <Search className="w-4 h-4 text-gray-600" />
            </button>
          ) : (
            <div className="bg-white border border-gray-200 rounded-lg shadow-sm flex items-center">
              <Search className="ml-3 w-4 h-4 text-gray-400" />
              <input
                type="text"
                placeholder="Search steps..."
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                autoFocus
                className="px-3 py-2 text-sm focus:outline-none bg-transparent w-64"
              />
              <button
                onClick={() => {
                  setShowSearch(false);
                  setSearchTerm('');
                }}
                className="mr-2 text-gray-400 hover:text-gray-600"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          )}
        </div>
      </div>

      <ReactFlow
        nodes={filteredNodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        onNodeClick={(_, node) => {
          setSelectedNodeId(node.id);
          if (onNodeClick && node.data.executedStep) {
            onNodeClick(node.data.executedStep);
          }
        }}
        fitView
        minZoom={0.1}
        maxZoom={2}
        defaultViewport={{ x: 0, y: 0, zoom: 0.8 }}
        panOnScroll={true}
        zoomOnScroll={false}
        panOnDrag={true}
      >
        <Background color="#e2e8f0" gap={16} />
        <Controls className="bg-white border border-gray-200 rounded" />
        <MiniMap
          nodeColor={(node) => {
            const owner = node.data?.owner || 'unknown';
            const colors = PARTY_COLORS[owner] || DEFAULT_COLOR;
            // Use execution status for color if executed
            if (node.data?.executionStatus && node.data.executionStatus !== 'not_executed') {
              switch (node.data.executionStatus) {
                case 'success': return '#22c55e';
                case 'failed': return '#ef4444';
                case 'violated': return '#f97316';
                case 'in_progress': return '#3b82f6';
                default: return '#9ca3af';
              }
            }
            return colors.border.replace('border-', '#');
          }}
          className="bg-white border border-gray-200 rounded"
        />
      </ReactFlow>
    </div>
  );
}

import { useEffect, useMemo, useState } from "react";
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
import { Clock, CheckCircle2, AlertCircle, Search, X, ChevronRight, AlertTriangle, Hash, Code, Copy, Check } from 'lucide-react';
import RightSidebar from './RightSidebar';

interface ContractGraphViewProps {
  contractName: string;
  version: number;
  graphData: {
    graph: {
      nodes: Record<string, {
        id: string;
        owner: string;
        type: string;
        mode?: string;
        required: boolean;
        next: string[];
        timeout?: string;
        maxDuration?: string;
        businessContext?: any;
      }>;
      entryPoints: string[];
      terminalSteps: string[];
      validation?: {
        has_cycles?: boolean;
        unreachable_steps?: string[];
        missing_transitions?: string[];
      };
    };
    transitions: Array<{
      From: string;
      To: string[];
      CanRetry?: boolean;
      MaxRetries?: number;
    }>;
    parties: string[];
  };
}

// Color palette for different parties
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

// Custom node component - matches thread graph design
function ContractNode({ data }: { data: any }) {
  const colors = PARTY_COLORS[data.owner] || DEFAULT_COLOR;
  const isEntryPoint = data.isEntryPoint;
  const isTerminal = data.isTerminal;

  return (
    <>
      {/* Handles for edge connections */}
      <Handle type="target" position={Position.Top} style={{ visibility: 'hidden' }} />
      <Handle type="source" position={Position.Bottom} style={{ visibility: 'hidden' }} />
      
      <div className={`rounded-xl shadow-md min-w-[200px] max-w-[240px] border-2 relative ${colors.bg} ${colors.border}`}>
        
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
            {isEntryPoint && '⭐'}
          </div>
        )}

        {/* Main Content */}
        <div className="px-4 pt-4 pb-3">
          {/* Step Name */}
          <div className="text-base font-semibold text-gray-900 mb-2">
            {data.label}
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
          </div>
        </div>
      </div>
    </>
  );
}

const nodeTypes = {
  contractNode: ContractNode,
};

// Custom hierarchical layout using BFS traversal
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

export default function ContractGraphView({ contractName, version, graphData }: ContractGraphViewProps) {
  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [showSearch, setShowSearch] = useState(false);
  const [showDebug, setShowDebug] = useState(false);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [showValidation, setShowValidation] = useState(false);
  const [showContext, setShowContext] = useState(false);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  // Build transition map for retry info
  const transitionMap = useMemo(() => {
    const map = new Map<string, { canRetry: boolean; maxRetries: number }>();
    graphData.transitions?.forEach((t) => {
      t.To.forEach((target: string) => {
        map.set(`${t.From}-${target}`, { canRetry: t.CanRetry || false, maxRetries: t.MaxRetries || 0 });
      });
    });
    return map;
  }, [graphData.transitions]);

  useEffect(() => {
    if (!graphData?.graph?.nodes) return;

    // Convert nodes map to array
    const nodesArray = Object.values(graphData.graph.nodes);

    // Create nodes
    const initialNodes: Node[] = nodesArray.map((node: any) => ({
      id: node.id,
      type: 'contractNode',
      position: { x: 0, y: 0 }, // Will be set by dagre
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
      },
    }));

    // Create edges from transitions array (node.next is empty in the data)
    const initialEdges: Edge[] = [];
    graphData.transitions?.forEach((transition: any) => {
      transition.To?.forEach((target: string) => {
        const transitionKey = `${transition.From}-${target}`;
        const retryInfo = transitionMap.get(transitionKey);

        initialEdges.push({
          id: transitionKey,
          source: transition.From,
          target: target,
          type: 'default',
          animated: false,
          style: {
            stroke: '#94a3b8',
            strokeWidth: 2,
            strokeDasharray: retryInfo?.canRetry ? '5,5' : undefined,
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
          labelBgPadding: [8, 4],
          labelBgBorderRadius: 4,
          markerEnd: {
            type: MarkerType.ArrowClosed,
            color: '#94a3b8',
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
  }, [graphData, transitionMap, setNodes, setEdges]);

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

  const selectedNodeData = useMemo(() => {
    if (!selectedNode) return null;
    return graphData.graph.nodes[selectedNode.id];
  }, [selectedNode, graphData]);

  // Get validation violations
  const validationViolations = useMemo(() => {
    const violations: Array<{ type: string; message: string }> = [];
    
    if (graphData.graph.validation) {
      const validation = graphData.graph.validation;
      
      // Check for cycles
      if (validation.has_cycles) {
        violations.push({
          type: 'Cycle Detected',
          message: 'The contract graph contains cycles which may cause infinite loops'
        });
      }
      
      // Check for unreachable steps
      if (validation.unreachable_steps && validation.unreachable_steps.length > 0) {
        violations.push({
          type: 'Unreachable Steps',
          message: `${validation.unreachable_steps.length} step(s) cannot be reached from entry points`
        });
      }
      
      // Check for missing transitions
      if (validation.missing_transitions && validation.missing_transitions.length > 0) {
        violations.push({
          type: 'Missing Transitions',
          message: `${validation.missing_transitions.length} step(s) have no outgoing transitions`
        });
      }
    }
    
    return violations;
  }, [graphData]);

  return (
    <div className="w-full h-[800px] bg-white border border-gray-200 rounded relative">
      {/* Header with parties and controls */}
      <div className="absolute top-4 left-4 right-4 z-10 flex items-center justify-between gap-4">
        {/* Left: Parties legend (scrollable if many) */}
        <div className="bg-white border border-gray-200 rounded-lg px-4 py-2 shadow-sm max-w-md">
          <div className="flex items-center gap-4 overflow-x-auto">
            <span className="text-xs font-semibold text-gray-700 whitespace-nowrap">Parties:</span>
            <div className="flex items-center gap-3 overflow-x-auto pb-1">
              {graphData.parties?.map((party) => {
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

          {/* Debug toggle */}
          <button
            onClick={() => setShowDebug(!showDebug)}
            className={`bg-white border border-gray-200 rounded-lg px-3 py-2 shadow-sm hover:bg-gray-50 transition-colors text-xs font-medium ${
              showDebug ? 'bg-blue-50 border-blue-300 text-blue-700' : 'text-gray-600'
            }`}
          >
            Debug
          </button>
        </div>
      </div>

      {/* Debug panel */}
      {showDebug && (
        <div className="absolute top-20 right-4 z-10 bg-white border border-gray-200 rounded-lg shadow-lg p-4 max-w-2xl max-h-96 overflow-auto">
          <h4 className="text-sm font-semibold text-gray-900 mb-2">Contract Graph Data</h4>
          <pre className="text-xs text-gray-700 overflow-auto">{JSON.stringify(graphData, null, 2)}</pre>
        </div>
      )}

      <ReactFlow
        nodes={filteredNodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        onNodeClick={(_, node) => setSelectedNodeId(node.id)}
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
            return colors.border.replace('border-', '#');
          }}
          className="bg-white border border-gray-200 rounded"
        />
      </ReactFlow>

      {/* Right Sidebar - Step Details */}
      {selectedNode && selectedNodeData && (
        <RightSidebar
          isOpen={true}
          onClose={() => {
            setSelectedNodeId(null);
            setShowContext(false);
          }}
          title={selectedNode.data.label}
          width="md"
        >
          <ContractStepDetail
            stepData={selectedNodeData}
            showContext={showContext}
            onToggleContext={() => setShowContext(!showContext)}
            copiedField={copiedField}
            onCopy={(text: string, field: string) => {
              navigator.clipboard.writeText(text);
              setCopiedField(field);
              setTimeout(() => setCopiedField(null), 2000);
            }}
            onNavigateToStep={(stepId: string) => setSelectedNodeId(stepId)}
          />
        </RightSidebar>
      )}
    </div>
  );
}

// Contract Step Detail Component - Matches thread view style
function ContractStepDetail({
  stepData,
  showContext,
  onToggleContext,
  copiedField,
  onCopy,
  onNavigateToStep
}: {
  stepData: any;
  showContext: boolean;
  onToggleContext: () => void;
  copiedField: string | null;
  onCopy: (text: string, field: string) => void;
  onNavigateToStep: (stepId: string) => void;
}) {
  const colors = PARTY_COLORS[stepData.owner] || DEFAULT_COLOR;
  
  return (
    <div className="space-y-6">
      {/* Step Name & Party */}
      <div>
        <div className="flex items-center gap-2 mb-3">
          <h4 className="text-2xl font-bold text-black">{stepData.id}</h4>
          <button
            onClick={() => onCopy(stepData.id, 'header')}
            className="p-1.5 rounded hover:bg-gray-100 transition-colors"
            title="Copy step name"
          >
            {copiedField === 'header' ? (
              <Check className="w-4 h-4 text-green-600" />
            ) : (
              <Copy className="w-4 h-4 text-gray-500" />
            )}
          </button>
        </div>
        
        {/* Party Badge */}
        <div className="flex items-center gap-2">
          <span className={`inline-flex items-center gap-2 px-3 py-1.5 rounded-full ${colors.bg} border ${colors.border}`}>
            <div className={`w-2 h-2 rounded-full ${colors.border.replace('border-', 'bg-')}`} />
            <span className={`text-xs font-medium ${colors.text}`}>{stepData.owner}</span>
          </span>
          
          {/* Required Badge */}
          {stepData.required && (
            <span className="inline-flex px-2 py-0.5 text-xs font-medium rounded-full bg-blue-100 text-blue-800">
              Required
            </span>
          )}
        </div>
      </div>

      {/* Timeout */}
      {stepData.timeout && (
        <div className="space-y-2">
          <h5 className="font-bold text-gray-900 flex items-center gap-2">
            <Clock className="w-4 h-4" />
            Timeout
          </h5>
          <div className="bg-amber-50 border border-amber-200 p-3 rounded">
            <div className="font-medium text-amber-900">{stepData.timeout}</div>
          </div>
        </div>
      )}

      {/* Max Duration */}
      {stepData.maxDuration && (
        <div className="space-y-2">
          <h5 className="font-bold text-gray-900 flex items-center gap-2">
            <Clock className="w-4 h-4" />
            Max Duration
          </h5>
          <div className="bg-blue-50 border border-blue-200 p-3 rounded">
            <div className="font-medium text-blue-900">{stepData.maxDuration}</div>
          </div>
        </div>
      )}

      {/* Type & Mode */}
      <div className="space-y-3">
        <h5 className="font-bold text-gray-900 flex items-center gap-2">
          <Hash className="w-4 h-4" />
          Properties
        </h5>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-600">Type:</span>
            <span className="font-medium">{stepData.type}</span>
          </div>
          {stepData.mode && (
            <div className="flex justify-between">
              <span className="text-gray-600">Mode:</span>
              <span className="font-medium">{stepData.mode}</span>
            </div>
          )}
        </div>
      </div>

      {/* Next Steps */}
      {stepData.next && stepData.next.length > 0 && (
        <div className="space-y-2">
          <h5 className="font-bold text-gray-900 flex items-center gap-2">
            <ChevronRight className="w-4 h-4" />
            Next Steps
          </h5>
          <div className="space-y-2">
            {stepData.next.map((nextStep: string) => (
              <button
                key={nextStep}
                onClick={() => onNavigateToStep(nextStep)}
                className="w-full text-left px-3 py-2 text-sm bg-gray-50 hover:bg-gray-100 rounded border border-gray-200 flex items-center justify-between transition-colors"
              >
                <span className="font-medium">{nextStep}</span>
                <ChevronRight className="w-4 h-4 text-gray-400" />
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Business Context */}
      {stepData.businessContext && (
        <div className="border-t border-gray-200 pt-6">
          <button
            onClick={onToggleContext}
            className="w-full px-3 py-3 text-left transition-colors group hover:bg-gray-50"
          >
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Code className="w-4 h-4 text-gray-600" />
                <span className="text-sm font-medium text-gray-900">
                  {showContext ? 'Hide' : 'Show'} Business Context
                </span>
              </div>
              <ChevronRight className={`w-4 h-4 text-gray-400 transition-transform ${showContext ? 'rotate-90' : ''}`} />
            </div>
          </button>
          
          {showContext && (
            <div className="mt-3 px-3 space-y-4">
              {/* Required Fields */}
              {stepData.businessContext.required && stepData.businessContext.required.length > 0 && (
                <div>
                  <h6 className="text-xs font-semibold text-gray-500 uppercase mb-2">Required Fields</h6>
                  <div className="space-y-1">
                    {stepData.businessContext.required.map((field: string) => (
                      <div key={field} className="flex items-center gap-2 text-sm">
                        <div className="w-1.5 h-1.5 rounded-full bg-red-500" />
                        <span className="font-mono text-gray-800">{field}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              
              {/* Optional Fields */}
              {stepData.businessContext.optional && stepData.businessContext.optional.length > 0 && (
                <div>
                  <h6 className="text-xs font-semibold text-gray-500 uppercase mb-2">Optional Fields</h6>
                  <div className="space-y-1">
                    {stepData.businessContext.optional.map((field: string) => (
                      <div key={field} className="flex items-center gap-2 text-sm">
                        <div className="w-1.5 h-1.5 rounded-full bg-gray-400" />
                        <span className="font-mono text-gray-600">{field}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
              
              {/* If there are other properties, show them as JSON */}
              {Object.keys(stepData.businessContext).some(key => key !== 'required' && key !== 'optional') && (
                <div>
                  <h6 className="text-xs font-semibold text-gray-500 uppercase mb-2">Additional Context</h6>
                  <div className="bg-gray-50 border border-gray-200 rounded p-3">
                    <pre className="text-xs font-mono text-gray-800 overflow-auto">
                      {JSON.stringify(
                        Object.fromEntries(
                          Object.entries(stepData.businessContext).filter(([key]) => key !== 'required' && key !== 'optional')
                        ),
                        null,
                        2
                      )}
                    </pre>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

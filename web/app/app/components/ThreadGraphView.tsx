import { useEffect, useRef, useState } from 'react';
import { type StepStateInfo } from '~/lib/graphql';
import { CheckCircle2, XCircle, Clock, AlertCircle, X, Calendar, Hash, RefreshCw, ChevronRight } from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';

interface ThreadGraphViewProps {
  steps: StepStateInfo[];
  validations?: any[];
}

// Client-side only check
const isClient = typeof window !== 'undefined';

interface GraphNode {
  id: string;
  label: string;
  status: string;
  x: number;
  y: number;
  retryCount: number;
}

interface GraphEdge {
  from: string;
  to: string;
}

export default function ThreadGraphView({ steps, validations = [] }: ThreadGraphViewProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
  const [hoveredNode, setHoveredNode] = useState<GraphNode | null>(null);
  const [sidePanelOpen, setSidePanelOpen] = useState(false);

  // Build graph structure from steps
  const { nodes, edges } = buildGraph(steps);
  
  // Get full step details for selected node
  const selectedStep = selectedNode 
    ? steps.find(s => `${s.stepName}:${s.idempotencyKey}` === selectedNode.id)
    : null;
    
  // Get validations for selected step
  const stepValidations = selectedStep && validations
    ? validations.filter(v => v.stepName === selectedStep.stepName && v.idempotencyKey === selectedStep.idempotencyKey)
    : [];

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // Set canvas size
    const rect = canvas.getBoundingClientRect();
    canvas.width = rect.width * window.devicePixelRatio;
    canvas.height = rect.height * window.devicePixelRatio;
    ctx.scale(window.devicePixelRatio, window.devicePixelRatio);

    // Clear canvas
    ctx.clearRect(0, 0, rect.width, rect.height);

    // Draw edges first (so they appear behind nodes)
    edges.forEach((edge) => {
      const fromNode = nodes.find((n) => n.id === edge.from);
      const toNode = nodes.find((n) => n.id === edge.to);
      if (!fromNode || !toNode) return;

      drawEdge(ctx, fromNode, toNode);
    });

    // Draw nodes
    nodes.forEach((node) => {
      const isSelected = selectedNode?.id === node.id;
      const isHovered = hoveredNode?.id === node.id;
      drawNode(ctx, node, isSelected, isHovered);
    });
  }, [nodes, edges, selectedNode, hoveredNode]);

  const handleCanvasClick = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;

    // Find clicked node
    const clickedNode = nodes.find((node) => {
      const dx = x - node.x;
      const dy = y - node.y;
      return Math.sqrt(dx * dx + dy * dy) < 40; // Node radius
    });

    if (clickedNode) {
      setSelectedNode(clickedNode);
      setSidePanelOpen(true);
    } else {
      setSelectedNode(null);
      setSidePanelOpen(false);
    }
  };

  const handleCanvasMouseMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const rect = canvas.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const y = e.clientY - rect.top;

    // Find hovered node
    const hovered = nodes.find((node) => {
      const dx = x - node.x;
      const dy = y - node.y;
      return Math.sqrt(dx * dx + dy * dy) < 40;
    });

    setHoveredNode(hovered || null);
    canvas.style.cursor = hovered ? 'pointer' : 'default';
  };

  // Don't render canvas during SSR
  if (!isClient) {
    return (
      <div className="flex items-center justify-center h-[600px] border-2 border-gray-200 rounded-lg bg-gray-50">
        <div className="text-gray-500">Loading graph...</div>
      </div>
    );
  }

  if (steps.length === 0) {
    return (
      <div className="flex items-center justify-center h-96 text-gray-500">
        <div className="text-center">
          <Clock className="w-12 h-12 mx-auto mb-3 text-gray-400" />
          <p>No steps to visualize</p>
        </div>
      </div>
    );
  }

  return (
    <div className="relative">
      <canvas
        ref={canvasRef}
        onClick={handleCanvasClick}
        onMouseMove={handleCanvasMouseMove}
        className="w-full h-[600px] border-2 border-gray-200 rounded-lg bg-gray-50"
      />

      {/* Legend */}
      <div className="absolute top-4 right-4 bg-white border-2 border-black p-4 shadow-lg">
        <h4 className="font-bold text-sm mb-2">Status</h4>
        <div className="space-y-2 text-xs">
          <div className="flex items-center gap-2">
            <div className="w-4 h-4 rounded-full bg-green-500"></div>
            <span>Success</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-4 h-4 rounded-full bg-red-500"></div>
            <span>Failed</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-4 h-4 rounded-full bg-blue-500"></div>
            <span>In Progress</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-4 h-4 rounded-full bg-gray-400"></div>
            <span>Pending</span>
          </div>
        </div>
      </div>

      {/* Side Panel */}
      {sidePanelOpen && selectedStep && (
        <div className="absolute top-0 right-0 h-full w-96 bg-white border-l-2 border-black shadow-2xl overflow-y-auto">
          {/* Panel Header */}
          <div className="sticky top-0 bg-black text-white p-4 flex items-center justify-between z-10">
            <h3 className="font-bold text-lg">Step Details</h3>
            <button
              onClick={() => {
                setSidePanelOpen(false);
                setSelectedNode(null);
              }}
              className="hover:bg-gray-800 p-1 rounded transition-colors"
            >
              <X className="w-5 h-5" />
            </button>
          </div>

          {/* Panel Content */}
          <div className="p-6 space-y-6">
            {/* Step Name & Status */}
            <div>
              <h4 className="text-2xl font-bold text-black mb-2">{selectedStep.stepName}</h4>
              <div className="flex items-center gap-2">
                {selectedStep.status === 'success' && <CheckCircle2 className="w-5 h-5 text-green-600" />}
                {selectedStep.status === 'failed' && <XCircle className="w-5 h-5 text-red-600" />}
                {selectedStep.status === 'in_progress' && <Clock className="w-5 h-5 text-blue-600" />}
                {selectedStep.status === 'pending' && <Clock className="w-5 h-5 text-gray-400" />}
                <span className={`px-3 py-1 text-sm font-medium rounded-full ${
                  selectedStep.status === 'success' ? 'bg-green-100 text-green-800' :
                  selectedStep.status === 'failed' ? 'bg-red-100 text-red-800' :
                  selectedStep.status === 'in_progress' ? 'bg-blue-100 text-blue-800' :
                  'bg-gray-100 text-gray-800'
                }`}>
                  {selectedStep.status}
                </span>
              </div>
            </div>

            {/* Retry Count */}
            {selectedStep.retryCount > 0 && (
              <div className="bg-orange-50 border-2 border-orange-200 p-4 rounded">
                <div className="flex items-center gap-2 text-orange-800">
                  <RefreshCw className="w-5 h-5" />
                  <span className="font-bold">Retried {selectedStep.retryCount} time{selectedStep.retryCount > 1 ? 's' : ''}</span>
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
                    <div className="font-medium">{formatDistanceToNow(new Date(selectedStep.firstSeenAt), { addSuffix: true })}</div>
                    <div className="text-xs text-gray-500">{new Date(selectedStep.firstSeenAt).toLocaleString()}</div>
                  </div>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">Last Updated:</span>
                  <div className="text-right">
                    <div className="font-medium">{formatDistanceToNow(new Date(selectedStep.lastUpdatedAt), { addSuffix: true })}</div>
                    <div className="text-xs text-gray-500">{new Date(selectedStep.lastUpdatedAt).toLocaleString()}</div>
                  </div>
                </div>
              </div>
            </div>

            {/* IDs */}
            <div className="space-y-3">
              <h5 className="font-bold text-gray-900 flex items-center gap-2">
                <Hash className="w-4 h-4" />
                Identifiers
              </h5>
              <div className="space-y-2 text-sm">
                <div>
                  <div className="text-gray-600 mb-1">Step ID:</div>
                  <div className="font-mono text-xs bg-gray-100 p-2 rounded break-all">
                    {selectedStep.latestStepID}
                  </div>
                </div>
                <div>
                  <div className="text-gray-600 mb-1">Idempotency Key:</div>
                  <div className="font-mono text-xs bg-gray-100 p-2 rounded break-all">
                    {selectedStep.idempotencyKey}
                  </div>
                </div>
                <div>
                  <div className="text-gray-600 mb-1">Thread ID:</div>
                  <div className="font-mono text-xs bg-gray-100 p-2 rounded break-all">
                    {selectedStep.threadId}
                  </div>
                </div>
              </div>
            </div>

            {/* Previous Step */}
            {selectedStep.previousStep && (
              <div className="space-y-2">
                <h5 className="font-bold text-gray-900 flex items-center gap-2">
                  <ChevronRight className="w-4 h-4 rotate-180" />
                  Previous Step
                </h5>
                <div className="bg-gray-50 border-2 border-gray-200 p-3 rounded">
                  <div className="font-medium">{selectedStep.previousStep}</div>
                </div>
              </div>
            )}

            {/* Validations */}
            {stepValidations.length > 0 && (
              <div className="space-y-3">
                <h5 className="font-bold text-gray-900 flex items-center gap-2">
                  <AlertCircle className="w-4 h-4" />
                  Validations ({stepValidations.length})
                </h5>
                <div className="space-y-2">
                  {stepValidations.map((validation, idx) => (
                    <div
                      key={idx}
                      className={`border-2 p-3 rounded ${
                        validation.hasCriticalViolation
                          ? 'bg-red-50 border-red-200'
                          : 'bg-yellow-50 border-yellow-200'
                      }`}
                    >
                      {validation.validations.map((issue: any, issueIdx: number) => (
                        <div key={issueIdx} className="text-sm">
                          <div className="font-medium text-gray-900">{issue.message}</div>
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
                  ))}
                </div>
              </div>
            )}

            {/* Verification Status */}
            {selectedStep.verified !== undefined && (
              <div className="space-y-2">
                <h5 className="font-bold text-gray-900">Cryptographic Verification</h5>
                <div className={`border-2 p-3 rounded ${
                  selectedStep.verified
                    ? 'bg-green-50 border-green-200'
                    : 'bg-red-50 border-red-200'
                }`}>
                  <div className="flex items-center gap-2">
                    {selectedStep.verified ? (
                      <CheckCircle2 className="w-5 h-5 text-green-600" />
                    ) : (
                      <XCircle className="w-5 h-5 text-red-600" />
                    )}
                    <span className="font-medium">
                      {selectedStep.verified ? 'Verified' : 'Verification Failed'}
                    </span>
                  </div>
                  {selectedStep.verificationError && (
                    <div className="mt-2 text-sm text-red-700">
                      {selectedStep.verificationError}
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// Build graph structure from steps
function buildGraph(steps: StepStateInfo[]): { nodes: GraphNode[]; edges: GraphEdge[] } {
  const nodes: GraphNode[] = [];
  const edges: GraphEdge[] = [];

  // Calculate layout (simple vertical flow)
  const nodeSpacing = 150;
  const startX = 100;
  const startY = 80;

  steps.forEach((step, index) => {
    const row = Math.floor(index / 3); // 3 nodes per row
    const col = index % 3;

    nodes.push({
      id: `${step.stepName}:${step.idempotencyKey}`,
      label: step.stepName,
      status: step.status,
      x: startX + col * 250,
      y: startY + row * nodeSpacing,
      retryCount: step.retryCount,
    });

    // Create edge from previous step
    if (step.previousStep && index > 0) {
      const previousStepNode = nodes.find((n) => n.label === step.previousStep);
      if (previousStepNode) {
        edges.push({
          from: previousStepNode.id,
          to: `${step.stepName}:${step.idempotencyKey}`,
        });
      }
    } else if (index > 0) {
      // If no explicit previous step, connect to the step before it
      edges.push({
        from: nodes[index - 1].id,
        to: `${step.stepName}:${step.idempotencyKey}`,
      });
    }
  });

  return { nodes, edges };
}

// Draw a node on the canvas
function drawNode(
  ctx: CanvasRenderingContext2D,
  node: GraphNode,
  isSelected: boolean,
  isHovered: boolean
) {
  const radius = 40;

  // Node color based on status
  const statusColors = {
    success: '#22c55e',
    failed: '#ef4444',
    in_progress: '#3b82f6',
    pending: '#9ca3af',
  };
  const color = statusColors[node.status as keyof typeof statusColors] || statusColors.pending;

  // Draw outer ring if selected or hovered
  if (isSelected || isHovered) {
    ctx.beginPath();
    ctx.arc(node.x, node.y, radius + 5, 0, 2 * Math.PI);
    ctx.strokeStyle = isSelected ? '#000000' : '#666666';
    ctx.lineWidth = 3;
    ctx.stroke();
  }

  // Draw node circle
  ctx.beginPath();
  ctx.arc(node.x, node.y, radius, 0, 2 * Math.PI);
  ctx.fillStyle = color;
  ctx.fill();
  ctx.strokeStyle = '#ffffff';
  ctx.lineWidth = 3;
  ctx.stroke();

  // Draw retry count badge if > 0
  if (node.retryCount > 0) {
    const badgeX = node.x + radius - 10;
    const badgeY = node.y - radius + 10;
    ctx.beginPath();
    ctx.arc(badgeX, badgeY, 12, 0, 2 * Math.PI);
    ctx.fillStyle = '#f97316';
    ctx.fill();
    ctx.fillStyle = '#ffffff';
    ctx.font = 'bold 10px sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(node.retryCount.toString(), badgeX, badgeY);
  }

  // Draw label below node
  ctx.fillStyle = '#000000';
  ctx.font = '12px sans-serif';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  
  // Wrap text if too long
  const maxWidth = 100;
  const words = node.label.split('_');
  let line = '';
  let y = node.y + radius + 10;

  words.forEach((word, i) => {
    const testLine = line + (line ? '_' : '') + word;
    const metrics = ctx.measureText(testLine);
    
    if (metrics.width > maxWidth && line) {
      ctx.fillText(line, node.x, y);
      line = word;
      y += 14;
    } else {
      line = testLine;
    }
  });
  ctx.fillText(line, node.x, y);
}

// Draw an edge between two nodes
function drawEdge(ctx: CanvasRenderingContext2D, from: GraphNode, to: GraphNode) {
  const fromRadius = 40;
  const toRadius = 40;

  // Calculate edge endpoints (on the circle perimeter)
  const angle = Math.atan2(to.y - from.y, to.x - from.x);
  const fromX = from.x + Math.cos(angle) * fromRadius;
  const fromY = from.y + Math.sin(angle) * fromRadius;
  const toX = to.x - Math.cos(angle) * toRadius;
  const toY = to.y - Math.sin(angle) * toRadius;

  // Draw line
  ctx.beginPath();
  ctx.moveTo(fromX, fromY);
  ctx.lineTo(toX, toY);
  ctx.strokeStyle = '#d1d5db';
  ctx.lineWidth = 2;
  ctx.stroke();

  // Draw arrowhead
  const arrowSize = 10;
  const arrowAngle = Math.PI / 6;
  
  ctx.beginPath();
  ctx.moveTo(toX, toY);
  ctx.lineTo(
    toX - arrowSize * Math.cos(angle - arrowAngle),
    toY - arrowSize * Math.sin(angle - arrowAngle)
  );
  ctx.moveTo(toX, toY);
  ctx.lineTo(
    toX - arrowSize * Math.cos(angle + arrowAngle),
    toY - arrowSize * Math.sin(angle + arrowAngle)
  );
  ctx.strokeStyle = '#d1d5db';
  ctx.lineWidth = 2;
  ctx.stroke();
}

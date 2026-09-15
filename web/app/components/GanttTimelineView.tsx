import { useMemo, useState, useRef, useEffect, useCallback } from 'react';
import { CheckCircle2, XCircle, Clock, AlertTriangle, RefreshCw, ZoomIn, ZoomOut, Search, X, XOctagon } from 'lucide-react';
import type { StepStateInfo } from '~/lib/graphql';

interface GanttTimelineViewProps {
  steps: StepStateInfo[];
  onStepClick: (step: StepStateInfo) => void;
  threadStatus?: string;
  selectedService?: string | null;
  onServiceSelect?: (service: string | null) => void;
}

// Row height — small gap above/below the bar
const ROW_HEIGHT = 76;
// Height of the coloured execution bar — tall enough to hold two text lines
const BAR_HEIGHT = 56;
// Minimum bar width — just enough to be visible and clickable for sub-ms steps
const MIN_BAR_WIDTH = 40;
// Max characters in the left gutter label
const MAX_LABEL_CHARS = 15;
// Max characters shown inside the bar itself
const BAR_LABEL_CHARS = 5;
// Fixed left label gutter width — fits 15-char names at 11px comfortably
const LABEL_WIDTH = 120;
// Right-side padding so the last bar isn't flush against the edge
const CHART_PADDING_RIGHT = 56;
// Gap between label gutter and first bar
const CHART_PADDING_LEFT = 5;

const STATUS_COLORS: Record<string, { bar: string; border: string; text: string; label: string }> = {
  success: { bar: '#dcfce7', border: '#16a34a', text: '#15803d', label: '#166534' },
  failed: { bar: '#fee2e2', border: '#dc2626', text: '#b91c1c', label: '#991b1b' },
  violated: { bar: '#ffedd5', border: '#ea580c', text: '#c2410c', label: '#9a3412' },
  in_progress: { bar: '#dbeafe', border: '#2563eb', text: '#1d4ed8', label: '#1e40af' },
  default: { bar: '#f3f4f6', border: '#9ca3af', text: '#6b7280', label: '#4b5563' },
};

function getStatusColors(status: string) {
  return STATUS_COLORS[status] ?? STATUS_COLORS.default;
}

// Service colour palette — same as the graph view for consistency
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

function getServiceColor(serviceName?: string) {
  if (!serviceName) return SERVICE_COLORS[0];
  let hash = 0;
  for (let i = 0; i < serviceName.length; i++) {
    hash = ((hash << 5) - hash) + serviceName.charCodeAt(i);
    hash = hash & hash;
  }
  return SERVICE_COLORS[Math.abs(hash) % SERVICE_COLORS.length];
}

function formatDuration(ms: number): string {
  if (ms <= 0) return '< 1ms';
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`;
  if (ms < 3_600_000) return `${(ms / 60_000).toFixed(2)}m`;
  return `${(ms / 3_600_000).toFixed(2)}h`;
}

function formatAxisTime(ms: number): string {
  if (ms === 0) return '0';
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(ms % 1000 === 0 ? 0 : 1)}s`;
  return `${(ms / 60_000).toFixed(1)}m`;
}

function getStatusIcon(status: string, size = 'w-3 h-3') {
  switch (status) {
    case 'success': return <CheckCircle2 className={size} />;
    case 'failed': return <XCircle className={size} />;
    case 'violated': return <AlertTriangle className={size} />;
    case 'in_progress': return <Clock className={`${size} animate-pulse`} />;
    default: return <Clock className={size} />;
  }
}

interface TooltipData {
  step: StepStateInfo;
  durationMs: number;
  x: number;
  y: number;
}

function Tooltip({ data }: { data: TooltipData }) {
  const colors = getStatusColors(data.step.status);
  const fmtTime = (iso: string) => {
    const d = new Date(iso);
    const base = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
    const ms = String(d.getMilliseconds()).padStart(3, '0');
    return `${base}.${ms}`;
  };
  const startTime = fmtTime(data.step.startedAt ?? data.step.firstSeenAt);
  const endTime = data.step.finishedAt ? fmtTime(data.step.finishedAt) : null;

  return (
    <div
      className="fixed z-50 pointer-events-none"
      style={{ left: data.x + 14, top: data.y - 12 }}
    >
      <div
        className="bg-gray-900 text-white rounded-lg shadow-xl p-3 text-xs space-y-1.5 w-52"
        style={{ border: `1px solid ${colors.border}` }}
      >
        <div className="font-semibold text-sm leading-snug break-words">{data.step.stepName}</div>
        <div className="flex items-center gap-1" style={{ color: colors.bar }}>
          {getStatusIcon(data.step.status)}
          <span className="capitalize">{data.step.status.replace('_', ' ')}</span>
          {data.step.retryCount > 1 && (
            <span className="flex items-center gap-0.5 text-orange-300 ml-1">
              <RefreshCw className="w-3 h-3" /> {data.step.retryCount}
            </span>
          )}
        </div>
        {data.step.actorService && (
          <div className="text-gray-400 truncate">{data.step.actorService}</div>
        )}
        <div className="border-t border-gray-700 pt-1.5 space-y-0.5 font-mono">
          <div><span className="text-gray-400">Start </span><span className="text-gray-200">{startTime}</span></div>
          {endTime && <div><span className="text-gray-400">End   </span><span className="text-gray-200">{endTime}</span></div>}
          <div><span className="text-gray-400">Dur   </span><span className="text-gray-200">{formatDuration(data.durationMs)}</span></div>
        </div>
      </div>
    </div>
  );
}

export default function GanttTimelineView({
  steps,
  onStepClick,
  threadStatus,
  selectedService = null,
  onServiceSelect,
}: GanttTimelineViewProps) {
  const chartScrollRef = useRef<HTMLDivElement>(null);
  const leftColRef = useRef<HTMLDivElement>(null);
  const stepBarRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const leftLabelRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const [containerWidth, setContainerWidth] = useState(900);
  const [tooltip, setTooltip] = useState<TooltipData | null>(null);
  const [zoom, setZoom] = useState(4);
  const [searchTerm, setSearchTerm] = useState('');
  const [showSearch, setShowSearch] = useState(false);
  const [statusFilters, setStatusFilters] = useState<Set<string>>(new Set(['success', 'failed', 'violated', 'in_progress']));

  useEffect(() => {
    const el = chartScrollRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => setContainerWidth(entry.contentRect.width));
    ro.observe(el);
    setContainerWidth(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const { sortedSteps, timelineStart, totalMs, searchResults } = useMemo(() => {
    if (steps.length === 0) return { sortedSteps: [], timelineStart: 0, totalMs: 1, searchResults: [] };

    // Sort all steps by time
    const sorted = [...steps].sort((a, b) =>
      new Date(a.startedAt ?? a.firstSeenAt).getTime() -
      new Date(b.startedAt ?? b.firstSeenAt).getTime()
    );

    // Apply status filters
    const filtered = sorted.filter(s => statusFilters.has(s.status));

    // Filter for search results dropdown (from filtered steps)
    let results: typeof steps = [];
    if (searchTerm) {
      results = filtered.filter(s =>
        (!selectedService || s.actorService === selectedService) &&
        (s.stepName.toLowerCase().includes(searchTerm.toLowerCase()) ||
          s.actorService?.toLowerCase().includes(searchTerm.toLowerCase()))
      );
    }

    // Calculate timeline range from filtered steps
    if (filtered.length === 0) return { sortedSteps: [], timelineStart: 0, totalMs: 1, searchResults: [] };

    const starts = filtered.map(s => new Date(s.startedAt ?? s.firstSeenAt).getTime());
    const ends = filtered.map(s => new Date(s.finishedAt ?? s.lastUpdatedAt).getTime());
    const start = Math.min(...starts);
    const end = Math.max(...ends, start + 1);
    return { sortedSteps: filtered, timelineStart: start, totalMs: end - start, searchResults: results };
  }, [steps, searchTerm, statusFilters, selectedService]);

  const services = useMemo(
    () => Array.from(new Set(sortedSteps.map(step => step.actorService).filter((service): service is string => !!service))),
    [sortedSteps],
  );

  const visibleStepCount = selectedService
    ? sortedSteps.filter(step => step.actorService === selectedService).length
    : sortedSteps.length;

  const toggleService = useCallback((service: string) => {
    onServiceSelect?.(selectedService === service ? null : service);
  }, [onServiceSelect, selectedService]);

  // Use sortedSteps and totalMs from the memoized calculation above.
  // The early return must be moved below all hook calls to avoid "Rendered more hooks than during previous render".

  const { usableBase, pxPerMs, chartMinWidth, majorTicks, minorTicks, intervalPx, finishedAtMarkers, gridBackground } = useMemo(() => {
    const usableBase = Math.max(200, containerWidth - CHART_PADDING_LEFT - CHART_PADDING_RIGHT);
    const pxPerMs = (usableBase / totalMs) * zoom;
    const chartMinWidth = Math.ceil(totalMs * pxPerMs) + CHART_PADDING_RIGHT;

    const TARGET_MAJOR_TICKS = 8;
    const rawInterval = totalMs / TARGET_MAJOR_TICKS;
    const mag = Math.pow(10, Math.floor(Math.log10(Math.max(rawInterval, 1))));
    const majorInterval = Math.ceil(rawInterval / mag) * mag;
    const minorInterval = majorInterval / 5;

    const majorTicks: number[] = [];
    const minorTicks: number[] = [];
    for (let t = 0; t <= totalMs + majorInterval; t += minorInterval) {
      if (t > totalMs * 1.02) break;
      if (Math.round(t / majorInterval) * majorInterval === Math.round(t)) {
        majorTicks.push(Math.round(t));
      } else {
        minorTicks.push(t);
      }
    }

    const intervalPx = Math.max(1, majorInterval * pxPerMs);

    const finishedAtMarkers = sortedSteps
      .filter(s => s.finishedAt)
      .map(s => ({
        ms: new Date(s.finishedAt!).getTime() - timelineStart,
        label: formatAxisTime(new Date(s.finishedAt!).getTime() - timelineStart),
      }));

    const gridBackground = `repeating-linear-gradient(to right, transparent, transparent ${intervalPx - 1}px, #d1d5db ${intervalPx - 1}px, #d1d5db ${intervalPx}px)`;

    return { usableBase, pxPerMs, chartMinWidth, majorTicks, minorTicks, intervalPx, finishedAtMarkers, gridBackground };
  }, [containerWidth, totalMs, zoom, sortedSteps, timelineStart]);

  const handleChartScroll = useCallback(() => {
    if (leftColRef.current && chartScrollRef.current) {
      leftColRef.current.scrollTop = chartScrollRef.current.scrollTop;
    }
  }, []);

  const handleLeftScroll = useCallback(() => {
    if (leftColRef.current && chartScrollRef.current) {
      chartScrollRef.current.scrollTop = leftColRef.current.scrollTop;
    }
  }, []);

  const scrollToStep = useCallback((step: StepStateInfo, scrollVertically: boolean = false) => {
    if (!chartScrollRef.current || !leftColRef.current) return;

    // Find the step's index in the sorted list
    const stepIndex = sortedSteps.findIndex(
      s => s.stepName === step.stepName && s.idempotencyKey === step.idempotencyKey
    );

    if (stepIndex === -1) return;

    // Calculate horizontal position based on step's start time (same as bar positioning)
    const startMs = new Date(step.startedAt ?? step.firstSeenAt).getTime() - timelineStart;
    const barLeft = CHART_PADDING_LEFT + (startMs * pxPerMs);
    const targetScrollLeft = barLeft - 50;

    if (scrollVertically) {
      // Calculate vertical position to center the row
      const RULER_HEIGHT = 48;
      const rowTop = stepIndex * ROW_HEIGHT + RULER_HEIGHT;
      const viewportHeight = chartScrollRef.current.clientHeight;
      const targetScrollTop = rowTop - (viewportHeight / 2) + (ROW_HEIGHT / 2);

      // Smooth scroll both horizontally and vertically
      chartScrollRef.current.scrollTo({
        left: Math.max(0, targetScrollLeft),
        top: Math.max(0, targetScrollTop),
        behavior: 'smooth'
      });

      // Also scroll the left column
      leftColRef.current.scrollTo({
        top: Math.max(0, targetScrollTop),
        behavior: 'smooth'
      });
    } else {
      // Smooth scroll horizontally only
      chartScrollRef.current.scrollTo({
        left: Math.max(0, targetScrollLeft),
        behavior: 'smooth'
      });
    }
  }, [sortedSteps, timelineStart, pxPerMs]);

  useEffect(() => {
    if (!selectedService) return;
    const firstServiceStep = sortedSteps.find(step => step.actorService === selectedService);
    if (firstServiceStep) scrollToStep(firstServiceStep, true);
  }, [selectedService, sortedSteps, scrollToStep]);

  // Memoize search close handler
  const handleCloseSearch = useCallback(() => {
    setShowSearch(false);
    setSearchTerm('');
  }, []);

  // Memoize search result click handler - use existing scrollToStep with vertical scroll
  const handleSearchResultClick = useCallback((result: StepStateInfo) => {
    // Use the existing scrollToStep function with vertical scrolling enabled
    scrollToStep(result, true);

    setShowSearch(false);
    setSearchTerm('');
  }, [scrollToStep]);

  // Memoize zoom handlers
  const handleZoomOut = useCallback(() => {
    setZoom(z => Math.max(1, +(z / 1.5).toFixed(1)));
  }, []);

  const handleZoomIn = useCallback(() => {
    setZoom(z => Math.min(50, +(z * 1.5).toFixed(1)));
  }, []);

  if (steps.length === 0) {
    return (
      <div className="flex items-center justify-center h-64 text-gray-500 border-2 border-dashed border-gray-200 rounded-xl bg-gray-50">
        <div className="text-center">
          <Clock className="w-10 h-10 mx-auto mb-3 text-gray-300" />
          <p className="text-sm">No steps to display</p>
        </div>
      </div>
    );
  }

  return (
    <div className="bg-white rounded-xl border border-gray-200 shadow-sm overflow-hidden flex flex-col">
      {/* Header */}
      <div className="px-4 py-3 border-b border-gray-100 flex flex-col gap-3 flex-shrink-0 sm:px-5 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-gray-800">Execution Timeline</h2>
          <p className="text-xs text-gray-400 mt-0.5 font-mono">
            {selectedService ? `${visibleStepCount} of ${sortedSteps.length}` : sortedSteps.length} step{visibleStepCount !== 1 ? 's' : ''} · {formatDuration(totalMs)} total
          </p>
        </div>
        <div className="flex items-center justify-end gap-2 relative">
          {showSearch ? (
            <div className="relative">
              <div className="flex items-center gap-1 bg-gray-50 border border-gray-200 rounded-lg px-2 py-1">
                <Search className="w-4 h-4 text-gray-400" />
                <input
                  type="text"
                  placeholder="Search steps..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  autoFocus
                  className="min-w-0 px-2 py-1 text-sm focus:outline-none bg-transparent w-32 sm:w-48"
                />
                <button
                  onClick={handleCloseSearch}
                  className="text-gray-400 hover:text-gray-600"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>

              {/* Search Results Dropdown */}
              {searchTerm && (
                <div className="absolute top-full right-0 mt-1 bg-white border border-gray-200 rounded-lg shadow-xl z-[100] max-h-64 overflow-y-auto w-[min(300px,calc(100vw-3rem))]">
                  {searchResults.length > 0 ? (
                    searchResults.map((result, idx) => (
                      <button
                        key={`${result.stepName}:${result.idempotencyKey}`}
                        onClick={() => handleSearchResultClick(result)}
                        className="w-full text-left px-3 py-2 hover:bg-gray-50 border-b border-gray-100 last:border-b-0 transition-colors"
                      >
                        <div className="text-sm font-medium text-gray-900">{result.stepName}</div>
                        {result.actorService && (
                          <div className="text-xs text-gray-500">{result.actorService}</div>
                        )}
                      </button>
                    ))
                  ) : (
                    <div className="px-3 py-4 text-sm text-gray-500 text-center">
                      No steps found matching "{searchTerm}"
                    </div>
                  )}
                </div>
              )}
            </div>
          ) : (
            <button
              onClick={() => setShowSearch(true)}
              className="p-1.5 rounded-lg border border-gray-200 hover:bg-gray-50 transition-colors text-gray-600"
              title="Search steps"
            >
              <Search className="w-4 h-4" />
            </button>
          )}
          <span className="text-xs text-gray-400 font-mono">{zoom}×</span>
          <button
            onClick={handleZoomOut}
            className="p-1.5 rounded-lg border border-gray-200 hover:bg-gray-50 transition-colors text-gray-600 disabled:opacity-30"
            disabled={zoom <= 1}
            title="Zoom out"
          >
            <ZoomOut className="w-4 h-4" />
          </button>
          <button
            onClick={handleZoomIn}
            className="p-1.5 rounded-lg border border-gray-200 hover:bg-gray-50 transition-colors text-gray-600"
            title="Zoom in"
          >
            <ZoomIn className="w-4 h-4" />
          </button>
        </div>
      </div>

      {services.length > 0 && (
        <div className="flex items-center gap-2 overflow-x-auto border-b border-gray-100 bg-gray-50/70 px-4 py-2 sm:px-5">
          <span className="shrink-0 text-[10px] font-semibold uppercase tracking-wide text-gray-400">Services</span>
          {services.map(service => {
            const serviceColor = getServiceColor(service);
            const isSelected = selectedService === service;
            const isDimmed = !!selectedService && !isSelected;
            return (
              <button
                key={service}
                type="button"
                aria-pressed={isSelected}
                onClick={() => toggleService(service)}
                className={`shrink-0 rounded-full border px-2.5 py-1 text-[10px] font-mono transition-all ${isDimmed ? 'opacity-35' : 'opacity-100'}`}
                style={{
                  borderColor: serviceColor.border,
                  backgroundColor: isSelected ? serviceColor.light : serviceColor.bg,
                  color: serviceColor.text,
                  boxShadow: isSelected ? `0 0 0 1px ${serviceColor.border}` : undefined,
                }}
                title={isSelected ? `Show all services` : `Show only ${service}`}
              >
                {service}
              </button>
            );
          })}
          {selectedService && (
            <button
              type="button"
              onClick={() => onServiceSelect?.(null)}
              className="shrink-0 text-[10px] font-medium text-gray-500 underline underline-offset-2 hover:text-gray-800"
            >
              Clear
            </button>
          )}
        </div>
      )}

      {/* Body: fixed label column + scrollable chart */}
      <div className="flex h-[480px] overflow-hidden sm:h-[600px]">

        {/* ── LEFT: label column — scrolls vertically with chart ── */}
        <div
          ref={leftColRef}
          className="flex-shrink-0 overflow-y-auto overflow-x-hidden flex flex-col border-r-2 border-gray-500 scrollbar-hide"
          style={{ width: LABEL_WIDTH }}
          onScroll={handleLeftScroll}
        >
          {/* Spacer matching ruler height */}
          <div className="flex-shrink-0 bg-gray-100 border-b border-gray-200 sticky top-0 z-10" style={{ height: 48 }} />
          {/* Labels */}
          {sortedSteps.map(step => {
            const svc = getServiceColor(step.actorService);
            const stepKey = `lbl-${step.stepName}:${step.idempotencyKey}`;
            const isServiceMatch = !selectedService || step.actorService === selectedService;
            return (
              <div
                key={stepKey}
                ref={el => {
                  if (el) leftLabelRefs.current.set(stepKey, el);
                }}
                className="flex-shrink-0 flex flex-col items-start justify-center px-3 border-b border-gray-300 overflow-hidden cursor-pointer hover:bg-opacity-80 transition-all relative"
                style={{ height: ROW_HEIGHT, backgroundColor: svc.bg }}
                onClick={() => scrollToStep(step)}
                title="Click to scroll to step"
              >
                {/* Status icons - positioned at top */}
                {(step.status === 'violated' || step.status === 'failed' || step.retryCount > 1) && (
                  <div className={`absolute top-1 right-1 flex items-center gap-1 transition-opacity ${isServiceMatch ? 'opacity-100' : 'opacity-0'}`}>
                    {step.status === 'violated' && (
                      <div className="flex items-center justify-center bg-orange-500 text-white rounded-full p-1 shadow-sm">
                        <AlertTriangle className="w-3.5 h-3.5" />
                      </div>
                    )}
                    {step.status === 'failed' && (
                      <div className="flex items-center justify-center bg-red-500 text-white rounded-full p-1 shadow-sm">
                        <XOctagon className="w-3.5 h-3.5" />
                      </div>
                    )}
                    {step.retryCount > 1 && (
                      <div className="flex items-center gap-0.5 bg-orange-500 text-white rounded-full px-1.5 py-1 text-[9px] font-bold leading-none shadow-sm">
                        <RefreshCw className="w-3 h-3" />{step.retryCount}
                      </div>
                    )}
                  </div>
                )}

                <div className="flex flex-col">
                  <span
                    className={`text-[11px] font-semibold whitespace-nowrap cursor-pointer hover:underline transition-opacity ${isServiceMatch ? 'opacity-100' : 'opacity-0 pointer-events-none'}`}
                    style={{ color: step.status === 'failed' ? '#991b1b' : step.status === 'violated' ? '#9a3412' : step.status === 'success' ? '#166534' : '#374151' }}
                    onClick={(e) => {
                      e.stopPropagation();
                      onStepClick(step);
                    }}
                    title={step.stepName}
                  >
                    {step.stepName.length > MAX_LABEL_CHARS
                      ? step.stepName.slice(0, MAX_LABEL_CHARS) + '...'
                      : step.stepName}
                  </span>
                  {step.actorService && (
                    <button
                      type="button"
                      aria-pressed={selectedService === step.actorService}
                      className={`text-left text-[9px] font-mono whitespace-nowrap mt-0.5 hover:underline transition-opacity ${isServiceMatch ? 'opacity-80' : 'opacity-30'}`}
                      title={step.actorService}
                      style={{ color: svc.text }}
                      onClick={(event) => {
                        event.stopPropagation();
                        toggleService(step.actorService!);
                      }}
                    >
                      {step.actorService.length > MAX_LABEL_CHARS 
                        ? step.actorService.slice(0, MAX_LABEL_CHARS) + '...' 
                        : step.actorService}
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>

        {/* ── RIGHT: chart — scrolls both axes ── */}
        <div
          ref={chartScrollRef}
          className="flex-1 overflow-x-auto overflow-y-auto scrollbar-hide"
          onScroll={handleChartScroll}
        >
          <div style={{ minWidth: chartMinWidth }}>

            {/* Sticky ruler ticks */}
            <div className="sticky top-0 z-10 select-none bg-gray-50 border-b border-gray-200" style={{ height: 48 }}>
              <div className="relative h-full">
                {minorTicks.map(t => (
                  <div key={`minor-${t}`} className="absolute bottom-0"
                    style={{ left: CHART_PADDING_LEFT + (t * pxPerMs), width: 1, height: 6, backgroundColor: '#d1d5db' }} />
                ))}
                {majorTicks.map(t => (
                  <div key={`major-${t}`} className="absolute bottom-0 flex flex-col items-start" style={{ left: CHART_PADDING_LEFT + (t * pxPerMs) }}>
                    <span className="text-[10px] text-gray-400 font-mono whitespace-nowrap" style={{ marginBottom: 12, marginLeft: 2 }}>
                      {formatAxisTime(t)}
                    </span>
                    <div style={{ width: 1, height: 12, backgroundColor: '#9ca3af' }} />
                  </div>
                ))}
                {finishedAtMarkers.map((m, idx) => (
                  <div key={`fin-${idx}`} className="absolute bottom-0 flex flex-col items-start" style={{ left: CHART_PADDING_LEFT + (m.ms * pxPerMs) }}>
                    <span className="text-[9px] font-mono whitespace-nowrap" style={{ color: '#b45309', marginBottom: 12, marginLeft: 2 }}>
                      {m.label}
                    </span>
                    <div style={{ width: 1, height: 12, backgroundColor: '#f59e0b' }} />
                  </div>
                ))}
                <div className="absolute bottom-0 left-0 right-0 h-px bg-gray-200" />
              </div>
            </div>

            {/* Step rows */}
            {sortedSteps.map(step => {
              const startMs = new Date(step.startedAt ?? step.firstSeenAt).getTime() - timelineStart;
              const endMs = step.finishedAt
                ? new Date(step.finishedAt).getTime() - timelineStart
                : step.status === 'in_progress' ? totalMs : startMs;
              const durationMs = Math.max(0, endMs - startMs);
              const barLeft = CHART_PADDING_LEFT + (startMs * pxPerMs);
              const barWidth = Math.max(MIN_BAR_WIDTH, durationMs * pxPerMs);
              const colors = getStatusColors(step.status);
              const isInProgress = step.status === 'in_progress';
              const svc = getServiceColor(step.actorService);
              const isServiceMatch = !selectedService || step.actorService === selectedService;

              return (
                <div
                  key={`${step.stepName}:${step.idempotencyKey}`}
                  className="relative border-b border-gray-300 transition-colors"
                  style={{
                    height: ROW_HEIGHT,
                    backgroundColor: isServiceMatch ? svc.bg : '#f9fafb',
                    backgroundImage: gridBackground,
                  }}
                >
                  {finishedAtMarkers.map((m, idx) => (
                    <div key={`fline-${idx}`} className="absolute top-0 bottom-0"
                      style={{ left: CHART_PADDING_LEFT + (m.ms * pxPerMs), width: 1, backgroundColor: '#fcd34d', opacity: 0.6 }} />
                  ))}
                  <div
                    ref={el => {
                      if (el) stepBarRefs.current.set(`${step.stepName}:${step.idempotencyKey}`, el);
                    }}
                    className={`absolute rounded-lg cursor-pointer hover:opacity-85 transition-opacity overflow-hidden ${isServiceMatch ? 'opacity-100' : 'opacity-0 pointer-events-none'}`}
                    style={{
                      left: barLeft, top: (ROW_HEIGHT - BAR_HEIGHT) / 2,
                      width: barWidth, height: BAR_HEIGHT,
                      backgroundColor: colors.bar, border: `1.5px solid ${colors.border}`,
                    }}
                    onClick={() => onStepClick(step)}
                    onMouseEnter={e => setTooltip({
                      step,
                      durationMs: new Date(step.finishedAt ?? step.lastUpdatedAt).getTime() -
                        new Date(step.startedAt ?? step.firstSeenAt).getTime(),
                      x: e.clientX, y: e.clientY,
                    })}
                    onMouseMove={e => setTooltip(prev => prev ? { ...prev, x: e.clientX, y: e.clientY } : null)}
                    onMouseLeave={() => setTooltip(null)}
                  >
                    {isInProgress && (
                      <div className="absolute inset-0 overflow-hidden rounded-lg" style={{ opacity: 0.4 }}>
                        <div className="absolute inset-0 -translate-x-full"
                          style={{ animation: 'shimmer 1.8s infinite', background: 'linear-gradient(90deg, transparent, rgba(255,255,255,0.8), transparent)' }} />
                      </div>
                    )}
                    <div className="flex flex-col justify-center h-full px-2 gap-0.5 overflow-hidden">
                      <span className="text-[11px] font-semibold leading-tight select-none truncate" style={{ color: colors.label }}>
                        {step.stepName}
                      </span>
                      {step.actorService && (
                        <span className="text-[9px] font-mono select-none truncate leading-none" style={{ color: colors.text, opacity: 0.9 }}>
                          {step.actorService}
                        </span>
                      )}
                      <span className="text-[8px] font-mono select-none truncate" style={{ color: colors.text, opacity: 0.8 }}>
                        {formatDuration(durationMs)}
                      </span>
                    </div>
                  </div>
                </div>
              );
            })}

          </div>
        </div>
      </div>

      {/* Legend with Filters */}
      <div className="px-5 py-2 border-t border-gray-100 bg-gray-50 flex items-center gap-5 flex-wrap flex-shrink-0">
        {Object.entries({ success: 'Success', failed: 'Failed', violated: 'Violated', in_progress: 'In Progress' }).map(
          ([status, label]) => {
            const c = getStatusColors(status);
            const isChecked = statusFilters.has(status);
            return (
              <label
                key={status}
                className="flex items-center gap-1.5 text-xs text-gray-600 cursor-pointer hover:text-gray-900 transition-colors"
              >
                <input
                  type="checkbox"
                  checked={isChecked}
                  onChange={(e) => {
                    const newFilters = new Set(statusFilters);
                    if (e.target.checked) {
                      newFilters.add(status);
                    } else {
                      newFilters.delete(status);
                    }
                    setStatusFilters(newFilters);
                  }}
                  className="w-3.5 h-3.5 rounded border-gray-300 focus:ring-offset-0 focus:ring-1 cursor-pointer"
                  style={{
                    accentColor: c.border,
                  }}
                />
                {/* <div className="w-4 h-3 rounded-sm border" style={{ backgroundColor: c.bar, borderColor: c.border }} /> */}
                <span className={!isChecked ? 'opacity-50' : ''}>{label}</span>
              </label>
            );
          }
        )}
      </div>

      {tooltip && <Tooltip data={tooltip} />}
    </div>
  );
}

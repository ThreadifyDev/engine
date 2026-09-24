import { useState } from 'react';
import { ArrowDown, ArrowUp, BarChart3, ChevronDown, Columns2, Hash, LayoutList, LineChart, Plus, RotateCcw, Rows3, SlidersHorizontal, Trash2, X } from 'lucide-react';
import type { EntityTypeMetric } from '~/lib/api';
import { TabBar } from '~/components/TabBar';
import { sources, type ProfileView, type ViewSource } from './profile-view';

const input = 'mt-2 w-full rounded-lg border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-900 shadow-sm transition placeholder:text-gray-400 hover:border-gray-300 focus:border-gray-400 focus:outline-none focus:ring-2 focus:ring-gray-900/10';
const iconButton = 'rounded-md p-1.5 text-gray-400 transition hover:bg-gray-100 hover:text-gray-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-gray-500 disabled:cursor-not-allowed disabled:opacity-25';

function sourcePresentation(source: ViewSource) {
  if (source === 'deliveryTrendChart') return { Icon: LineChart, label: 'Line graph', description: 'Completed and failed deliveries over time.' };
  if (source === 'deliveryHealthChart') return { Icon: BarChart3, label: 'Bar chart', description: 'Compare completion, failure, error, and recovery rates.' };
  if (sources[source].kind === 'metric') return { Icon: Hash, label: 'Metric', description: sources[source].description };
  return { Icon: LayoutList, label: 'Section', description: sources[source].description };
}

export default function ProfileViewControls({ definition, selected, dirty, onChange, onSelect, onAdd, onMove, onRemove, onDiscard, onReset, metrics = [], onAddMetric }: {
  metrics?: EntityTypeMetric[]; onAddMetric?: (metric: EntityTypeMetric) => void;
  definition: ProfileView; selected: string; dirty: boolean;
  onChange: (definition: ProfileView) => void; onSelect: (id: string) => void;
  onAdd: (source: ViewSource) => void; onMove: (index: number, offset: number) => void;
  onRemove: (id: string) => void; onDiscard: () => void; onReset: () => void;
}) {
  const [tab, setTab] = useState<'content' | 'appearance'>('content');
  const [adding, setAdding] = useState(false);
  const available = (Object.keys(sources) as ViewSource[]).filter(source => source !== 'configuredMetric' && !definition.blocks.some(block => block.source === source));

  const availableMetrics = metrics.filter(metric => metric.id && !definition.blocks.some(block => block.metricId === metric.id));

  return <aside aria-label="View controls" className="min-w-0 overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
    <div className="px-5 pt-5">
      <div className="flex items-center gap-2.5"><SlidersHorizontal className="h-4 w-4 text-gray-400" /><h2 className="text-sm font-semibold text-gray-900">Presentation</h2></div>
      <p className="mt-2 text-xs leading-5 text-gray-500">Every metric is included automatically. Adjust its display and order.</p>
      <TabBar label="View customization" value={tab} onChange={setTab} panelId="view-controls-panel" className="mt-4" items={[{ value: 'content', label: <>Content <span className="rounded bg-stone-100 px-1.5 py-0.5 text-[10px] tabular-nums text-stone-500">{definition.blocks.length}</span></> }, { value: 'appearance', label: 'Appearance' }]} />
    </div>

    <div id="view-controls-panel" role="tabpanel" aria-label={tab === 'content' ? 'Content' : 'Appearance'} className="p-5">
      {tab === 'content' ? <div className="space-y-4">
        <div className="flex items-center justify-between gap-2"><h3 className="text-xs font-medium text-gray-500">Overview blocks</h3><span className="text-[11px] text-gray-400">Shown in this order</span></div>
        {!definition.blocks.length && <div className="rounded-lg border border-dashed border-gray-200 px-4 py-6 text-center"><LayoutList className="mx-auto mb-2 h-5 w-5 text-gray-300" /><p className="text-sm font-medium text-gray-700">Start with a block</p><p className="mt-1 text-xs leading-5 text-gray-500">Choose a metric, chart, or detail section.</p></div>}
        <ol className="space-y-3">
          {definition.blocks.map((block, index) => {
            const { Icon, label, description } = sourcePresentation(block.source);
            const expanded = selected === block.id;
            return <li key={block.id} className={`overflow-hidden rounded-lg border transition ${expanded ? 'border-gray-300 shadow-sm' : 'border-gray-200'}`}>
              <button type="button" aria-expanded={expanded} aria-controls={`block-settings-${block.id}`} onClick={() => onSelect(expanded ? '' : block.id)} className={`flex w-full min-w-0 items-center gap-3 p-3 text-left transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-gray-500 ${expanded ? 'bg-gray-50' : 'hover:bg-gray-50'}`}>
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-500"><Icon className="h-4 w-4" /></span>
                <span className="min-w-0 flex-1"><span className="block truncate text-xs font-semibold text-gray-800">{block.title || sources[block.source].label}</span><span className="mt-1 block text-[11px] text-gray-400">{label}</span></span>
                <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-gray-400 transition-transform ${expanded ? 'rotate-180' : ''}`} />
              </button>
              {expanded && <div id={`block-settings-${block.id}`} className="border-t border-gray-200 p-3">
                <label className="block text-xs font-medium text-gray-600">Block title<input className={input} maxLength={100} value={block.title} onChange={event => onChange({ ...definition, blocks: definition.blocks.map(item => item.id === block.id ? { ...item, title: event.target.value } : item) })} /></label>
                {block.source === 'configuredMetric' && <label className="mt-3 block text-xs font-medium text-gray-600">Display as<select aria-label="Metric presentation" className={input} value={block.display} onChange={event => onChange({ ...definition, blocks: definition.blocks.map(item => item.id === block.id ? { ...item, display: event.target.value as 'card' | 'table' | 'line' | 'bar' } : item) })}><option value="card">Card</option><option value="table">Table</option><option value="line">Line chart</option><option value="bar">Bar chart</option></select></label>}
                <p className="mt-3 text-xs leading-5 text-gray-500">{block.metricId ? metrics.find(metric => metric.id === block.metricId)?.name || metrics.find(metric => metric.id === block.metricId)?.custom_definition?.name || 'Configured metric reference' : description}</p>
              </div>}
              <div className="flex items-center justify-between border-t border-gray-100 px-2.5 py-1.5">
                <span className="pl-1 text-[10px] font-medium tabular-nums text-gray-400">{String(index + 1).padStart(2, '0')}</span>
                <div className="flex items-center gap-0.5"><button type="button" className={iconButton} aria-label={`Move ${block.title} up`} title="Move up" disabled={index === 0} onClick={() => onMove(index, -1)}><ArrowUp className="h-3.5 w-3.5" /></button><button type="button" className={iconButton} aria-label={`Move ${block.title} down`} title="Move down" disabled={index === definition.blocks.length - 1} onClick={() => onMove(index, 1)}><ArrowDown className="h-3.5 w-3.5" /></button>{block.source !== 'configuredMetric' && <><span className="mx-1 h-3 w-px bg-gray-200" /><button type="button" className={`${iconButton} hover:!bg-red-50 hover:!text-red-600`} aria-label={`Remove ${block.title}`} title="Remove block" onClick={() => onRemove(block.id)}><Trash2 className="h-3.5 w-3.5" /></button></>}</div>
              </div>
            </li>;
          })}
        </ol>

        {definition.blocks.length < 100 && (available.length > 0 || availableMetrics.length > 0) && (adding ? <div className="overflow-hidden rounded-lg border border-gray-200 bg-gray-50/50">
          <div className="flex items-center justify-between border-b border-gray-200 px-3 py-2"><h3 className="text-xs font-semibold text-gray-700">Choose a block</h3><button type="button" className={iconButton} aria-label="Close block picker" onClick={() => setAdding(false)}><X className="h-3.5 w-3.5" /></button></div>
          <div className="max-h-80 space-y-1 overflow-y-auto p-2">
            <p className="px-2 py-1 text-[11px] font-semibold text-gray-500">Your metric definitions</p>
            {availableMetrics.map(metric => <button type="button" key={metric.id} onClick={() => { onAddMetric?.(metric); setAdding(false); }} className="flex w-full items-center gap-2 rounded-md p-2.5 text-left text-xs font-medium text-gray-800 hover:bg-white"><Hash className="h-4 w-4 text-gray-400" />{metric.name || metric.custom_definition?.name || metric.template_id}</button>)}
            {!metrics.length && <p className="px-2 pb-3 text-xs leading-5 text-gray-400">Add and save a metric in Data & metrics to use it here.</p>}
            <p className="px-2 pt-3 text-[11px] font-semibold text-gray-500">Built-in profile data</p>
            {available.map(source => {
            const { Icon, description } = sourcePresentation(source);
            return <button type="button" key={source} onClick={() => { onAdd(source); setAdding(false); }} className="group flex w-full items-start gap-3 rounded-md p-2.5 text-left transition hover:bg-white hover:shadow-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-gray-500"><Icon className="mt-0.5 h-4 w-4 shrink-0 text-gray-400 group-hover:text-gray-800" /><span className="min-w-0 flex-1"><span className="block text-xs font-medium text-gray-800">{sources[source].label}</span><span className="mt-1 block text-[11px] leading-4 text-gray-500">{description}</span></span><Plus className="mt-0.5 h-3.5 w-3.5 shrink-0 text-gray-400" /></button>;
          })}</div>
        </div> : <button type="button" className="flex w-full items-center justify-center gap-2 rounded-lg border border-dashed border-gray-300 py-3 text-xs font-medium text-gray-600 transition hover:border-gray-400 hover:bg-gray-50 hover:text-gray-900 focus-visible:outline focus-visible:outline-2 focus-visible:outline-gray-500" onClick={() => setAdding(true)}><Plus className="h-4 w-4" />Add block</button>)}
        <p className="text-[11px] leading-5 text-gray-400">Changes appear in the preview. Thread history stays in its own tab.</p>
      </div> : <div className="space-y-6">
        <div className="space-y-4"><label className="block text-xs font-medium text-gray-600">View title<input className={input} maxLength={100} value={definition.title} onChange={event => onChange({ ...definition, title: event.target.value })} /></label><label className="block text-xs font-medium text-gray-600">Description <span className="font-normal text-gray-400">· optional</span><textarea aria-label="Description" className={`${input} resize-y leading-5`} maxLength={500} rows={3} placeholder="A little context for your team" value={definition.description} onChange={event => onChange({ ...definition, description: event.target.value })} /></label></div>
        <fieldset className="border-t border-gray-100 pt-5"><legend className="sr-only">Card columns</legend><p className="mb-3 text-xs font-medium text-gray-600">Card layout</p><div className="grid grid-cols-2 gap-2">{([1, 2] as const).map(columns => {
          const Icon = columns === 1 ? Rows3 : Columns2;
          return <button type="button" key={columns} aria-pressed={definition.columns === columns} onClick={() => onChange({ ...definition, columns })} className={`flex items-center justify-center gap-2 rounded-lg border px-3 py-3 text-xs font-medium transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-gray-500 ${definition.columns === columns ? 'border-gray-900 bg-gray-50 text-gray-950' : 'border-gray-200 text-gray-500 hover:border-gray-300'}`}><Icon className="h-4 w-4" />{columns === 1 ? 'One column' : 'Two columns'}</button>;
        })}</div><p className="mt-2 text-[11px] leading-5 text-gray-400">Charts and detail sections always use the full width.</p></fieldset>
        <fieldset><legend className="mb-3 text-xs font-medium text-gray-600">Default range</legend><div className="grid grid-cols-3 gap-1 rounded-lg bg-gray-100 p-1">{(['7d', '30d', '90d'] as const).map(range => <button type="button" key={range} aria-pressed={definition.range === range} onClick={() => onChange({ ...definition, range })} className={`rounded-md px-2 py-2 text-xs font-medium transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-gray-500 ${definition.range === range ? 'bg-white text-gray-950 shadow-sm ring-1 ring-black/5' : 'text-gray-500 hover:text-gray-800'}`}>{parseInt(range)} days</button>)}</div><p className="mt-2 text-[11px] leading-5 text-gray-400">Applies to charts, configured metrics, and delivery health.</p></fieldset>
      </div>}
    </div>
    <div className="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 bg-gray-50/70 px-5 py-3.5"><button type="button" disabled={!dirty} onClick={onDiscard} className="text-xs font-medium text-gray-500 transition hover:text-gray-900 disabled:opacity-35">Discard changes</button><button type="button" onClick={onReset} className="inline-flex items-center gap-1.5 text-xs text-gray-500 transition hover:text-gray-900"><RotateCcw className="h-3 w-3" />Use starter view</button></div>
  </aside>;
}

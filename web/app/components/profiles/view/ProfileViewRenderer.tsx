import type { EntityProfile } from '~/lib/api';
import ConfiguredMetricView from './ConfiguredMetricView';
import OverviewTab from '../OverviewTab';
import MetricsTab from '../MetricsTab';
import DeliveryHealthTab from '../DeliveryHealthTab';
import { sources, type ProfileView } from './profile-view';

export default function ProfileViewRenderer({ definition, profile, type, onRangeChange }: { definition: ProfileView; profile: EntityProfile | null; type: string; onRangeChange?: (range: ProfileView['range']) => void }) {
  return <div className="min-w-0 space-y-5">
    <div><h2 className="break-words text-xl font-semibold tracking-tight text-stone-900">{definition.title || 'Untitled view'}</h2>{definition.description && <p className="mt-2 break-words text-sm leading-relaxed text-stone-500">{definition.description}</p>}</div>
    <div className={`grid min-w-0 grid-cols-1 gap-4 ${definition.columns === 2 ? 'md:grid-cols-2' : ''}`}>
      {definition.blocks.map(block => {
        const source = sources[block.source];
        const metric = source.kind === 'metric';
        const value = metric && profile?.metrics ? profile.metrics[block.source as keyof typeof profile.metrics] : null;
        return <section key={`${block.id}:${profile?.id ?? 'empty'}:${definition.range}`} className={`min-w-0 rounded-2xl border border-stone-200 bg-white ${metric || (block.source === 'configuredMetric' && block.display === 'card') ? 'p-5' : 'p-4 sm:p-5 md:col-span-full'}`}>
          <h3 className="break-words text-sm font-medium text-stone-700">{block.title || source.label}</h3>
          {metric ? <>
            <p className="mt-3 text-3xl font-semibold tabular-nums tracking-tight text-stone-900">{typeof value === 'number' && Number.isFinite(value) ? `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })}${block.source === 'averageDeliveryTimeMs' ? ' ms' : ''}` : '—'}</p>
            <p className="mt-3 text-xs leading-relaxed text-stone-500">{profile ? typeof value === 'number' ? source.description : 'No data available for this metric.' : 'Select an entity to preview live data.'}</p>
          </> : !profile ? <div className="mt-4 rounded-xl border border-dashed border-stone-200 bg-stone-50 p-6 text-center"><p className="text-sm text-stone-500">{source.description}</p><p className="mt-2 text-xs text-stone-400">Live data will appear when an entity is selected.</p></div>
            : <div className="mt-4">
              {block.source === 'configuredMetric' && block.metricId && block.display && <ConfiguredMetricView companyId={profile.companyId} metricId={block.metricId} display={block.display} refKey={profile.refKey} type={type} range={definition.range} />}
              {block.source === 'details' && <OverviewTab hideHeading profile={profile} metrics={profile.metrics ?? {}} />}
              {block.source === 'computedMetrics' && <MetricsTab refKey={profile.refKey} type={type} initialRange={definition.range} selectedRange={onRangeChange ? definition.range : undefined} onRangeChange={onRangeChange} hasMetricsConfig={!!profile.profileType?.metricsConfig?.length} />}
              {block.source === 'deliveryHealth' && <DeliveryHealthTab refKey={profile.refKey} type={type} initialRange={definition.range} selectedRange={onRangeChange ? definition.range : undefined} onRangeChange={onRangeChange} />}
              {block.source === 'deliveryTrendChart' && <DeliveryHealthTab trendOnly refKey={profile.refKey} type={type} initialRange={definition.range} selectedRange={onRangeChange ? definition.range : undefined} onRangeChange={onRangeChange} />}
              {block.source === 'deliveryHealthChart' && <DeliveryHealthTab chartOnly refKey={profile.refKey} type={type} initialRange={definition.range} selectedRange={onRangeChange ? definition.range : undefined} onRangeChange={onRangeChange} />}
            </div>}
        </section>;
      })}
    </div>
    {profile && <p className="text-xs leading-relaxed text-stone-400">Summary cards use the profile’s reported metrics. The default time range applies to configured metrics and delivery health. Archived activity may take time to appear.{profile.metrics?.lastCalculatedAt && ` Summary updated ${new Date(profile.metrics.lastCalculatedAt).toLocaleString()}.`}</p>}
  </div>;
}

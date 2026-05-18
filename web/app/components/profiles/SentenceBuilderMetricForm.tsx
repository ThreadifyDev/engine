import { useState, useCallback, useMemo, useEffect } from 'react';
import { X, Plus, Save, Check } from 'lucide-react';
import { api, CustomMetricDefinition, CustomMetricFilter } from '~/lib/api';

interface SentenceBuilderMetricFormProps {
  onSubmit: (definition: CustomMetricDefinition) => void;
  onCancel?: () => void;
  initialData?: CustomMetricDefinition;
}

const OPERATIONS = ['COUNT', 'RATE', 'AVG', 'SUM', 'MIN', 'MAX'] as const;

const FIELD_OPTIONS: Record<string, string[]> = {
  COUNT: ['threads', 'steps', 'violations', 'retries', 'stepCount'],
  RATE: ['outcome', 'violations'],
  AVG: ['duration', 'retries'],
  SUM: ['violations', 'retries', 'duration', 'stepCount'],
  MIN: ['duration'],
  MAX: ['duration', 'retries'],
};

const FILTER_KEYS = [
  'step name',
  'step outcome',
  'actor',
  'actor service',
  'thread outcome',
  'process type',
  'violation type',
  'tags',
] as const;

const STEP_OUTCOMES = ['success', 'failed', 'error'];
const THREAD_OUTCOMES = ['success', 'failed', 'error', 'incomplete'];
const VIOLATION_TYPES = ['sla_breach', 'missing_step', 'wrong_sequence', 'unexpected_outcome', 'partner_silent'];

const GROUP_BY_OPTIONS = ['step name', 'outcome', 'process type', 'violation type', 'tag', 'none'] as const;
const VISUALISATION_OPTIONS = ['number', 'line', 'table', 'bar'] as const;

function getValidFields(operation: string): string[] {
  return FIELD_OPTIONS[operation] || [];
}

function getDefaultField(operation: string): string {
  const fields = getValidFields(operation);
  return fields[0] || '';
}

function getFilterValueOptions(key: string): string[] | null {
  switch (key) {
    case 'step outcome': return STEP_OUTCOMES;
    case 'thread outcome': return THREAD_OUTCOMES;
    case 'violation type': return VIOLATION_TYPES;
    default: return null;
  }
}

function getVisualisationOptions(groupBy: string): string[] {
  if (!groupBy || groupBy === 'none') return ['number'];
  return ['table', 'bar'];
}

function deriveTargetFromField(field: string): 'thread' | 'step' {
  if (field === 'steps' || field === 'stepCount' || field === 'retries' || field === 'duration') {
    return 'step';
  }
  return 'thread';
}

function calculateComplexity(def: CustomMetricDefinition): number {
  let score = 1;
  if (def.filters && def.filters.length > 0) {
    score += def.filters.length;
  }
  if (def.group_by && def.group_by !== 'none') {
    score += 2;
  }
  if (def.field === 'violations') {
    score += 2;
  }
  // step join needed when field is steps or step-related on thread target
  const target = deriveTargetFromField(def.field);
  if (target === 'thread' && (def.field === 'steps' || def.field === 'stepCount' || def.field === 'retries')) {
    score += 2;
  }
  return score;
}

export default function SentenceBuilderMetricForm({
  onSubmit,
  onCancel,
  initialData,
}: SentenceBuilderMetricFormProps) {
  const [definition, setDefinition] = useState<CustomMetricDefinition>({
    name: '',
    operation: 'COUNT',
    field: 'threads',
    filters: [],
    group_by: 'none',
    visualisation: 'number',
    step_name: '',
  });

  // Initialize form with initialData when provided
  useEffect(() => {
    if (initialData) {
      setDefinition(initialData);
    }
  }, [initialData]);

  const [costPerComplexity, setCostPerComplexity] = useState<number>(1);
  const [justSaved, setJustSaved] = useState(false);

  const updateDefinition = useCallback((updates: Partial<CustomMetricDefinition>) => {
    setDefinition(prev => {
      const next = { ...prev, ...updates };

      // Auto-adjust field when operation changes
      if (updates.operation && updates.operation !== prev.operation) {
        const validFields = getValidFields(updates.operation);
        if (!validFields.includes(next.field || '')) {
          next.field = validFields[0] || '';
        }
      }

      // Auto-derive target from field
      if (updates.field) {
        next.target = deriveTargetFromField(next.field);
        
        // Remove step filters and invalid group by if target changes to thread
        if (next.target === 'thread') {
          if (next.filters) {
            next.filters = next.filters.filter(f => !['step name', 'step outcome', 'actor', 'actor service'].includes(f.key));
          }
          if (['step name', 'outcome', 'actor', 'actor service'].includes(next.group_by || 'none')) {
            next.group_by = 'none';
          }
        }
      }

      // Auto-adjust visualisation when groupBy changes — commented out for now
      /*
      if (updates.group_by !== undefined && updates.group_by !== prev.group_by) {
        const visOptions = getVisualisationOptions(next.group_by || 'none');
        if (!visOptions.includes(next.visualisation || '')) {
          next.visualisation = visOptions[0];
        }
        if (next.group_by === 'none') {
          next.granularity = undefined;
        }
      }
      */

      return next;
    });
  }, []);

  // Fetch pricing config on mount
  useEffect(() => {
    api.getPricing().then(data => {
      const cost = data.credit?.custom_metric_cost_per_complexity_millicents;
      if (cost !== undefined && cost > 0) {
        setCostPerComplexity(cost);
      }
    }).catch(() => {
      // fallback to default
    });
  }, []);

  const complexity = useMemo(() => calculateComplexity(definition), [definition]);
  const estimatedCostCents = ((complexity * costPerComplexity) / 1000).toFixed(3);

  const addFilter = () => {
    updateDefinition({
      filters: [...(definition.filters || []), { key: '', value: '' }],
    });
  };

  const updateFilter = (index: number, key: string, value: string) => {
    const newFilters = [...(definition.filters || [])];
    newFilters[index] = { key, value };
    updateDefinition({ filters: newFilters });
  };

  const removeFilter = (index: number) => {
    const newFilters = [...(definition.filters || [])];
    newFilters.splice(index, 1);
    updateDefinition({ filters: newFilters });
  };

  const getDefinitionWithTarget = (): CustomMetricDefinition => ({
    ...definition,
    target: deriveTargetFromField(definition.field),
  });

  const handleSave = () => {
    if (!definition.name || !definition.operation || !definition.field) return;
    onSubmit(getDefinitionWithTarget());
    setJustSaved(true);
    setTimeout(() => setJustSaved(false), 1200);
  };

  const validFields = getValidFields(definition.operation || 'COUNT');
  // const visOptions = getVisualisationOptions(definition.group_by || 'none'); // commented out for now
  const target = deriveTargetFromField(definition.field);

  const availableFilterKeys = FILTER_KEYS.filter(k => {
    if (target === 'thread' && ['step name', 'step outcome', 'actor', 'actor service'].includes(k)) {
      return false;
    }
    return true;
  });

  // 'step name' and 'outcome' are valid for both targets (step name maps to
  // s.step_name for step target, t.contract_name for thread target).
  // 'violation type' requires the validations join — keep it available always.
  // 'process type' groups by t.contract_name — valid for both targets.
  // 'period' is always valid.
  const availableGroupByOptions = GROUP_BY_OPTIONS;

  return (
    <div className="space-y-5">
      {/* Metric Name */}
      <div>
        <label className="block text-sm font-medium text-gray-700 mb-1">Metric name</label>
        <input
          type="text"
          value={definition.name}
          onChange={e => updateDefinition({ name: e.target.value })}
          placeholder="e.g. Failed Payment Rate"
          className="w-full px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none"
          required
        />
      </div>

      {/* Sentence Builder */}
      <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
        <div className="flex flex-wrap items-center gap-2 text-sm text-gray-800 leading-relaxed">
          <span className="text-gray-500">I want to see the</span>
          
          <select
            value={definition.operation}
            onChange={e => updateDefinition({ operation: e.target.value as any })}
            className="bg-white border border-gray-300 rounded px-2 py-1 text-sm font-medium focus:ring-1 focus:ring-black focus:border-black outline-none"
          >
            {OPERATIONS.map(op => (
              <option key={op} value={op}>{op}</option>
            ))}
          </select>

          <span className="text-gray-500">of</span>

          <select
            value={definition.field}
            onChange={e => updateDefinition({ field: e.target.value })}
            className="bg-white border border-gray-300 rounded px-2 py-1 text-sm font-medium focus:ring-1 focus:ring-black focus:border-black outline-none"
          >
            {validFields.map(f => (
              <option key={f} value={f}>{f}</option>
            ))}
          </select>
        </div>

        {/* Filters */}
        <div className="mt-3 space-y-2">
          {(definition.filters || []).map((filter, index) => (
            <div key={index} className="flex items-center gap-2">
              <span className="text-xs text-gray-500 w-8 text-right">
                {index === 0 ? 'where' : 'and'}
              </span>
              <select
                value={filter.key}
                onChange={e => updateFilter(index, e.target.value, '')}
                className="bg-white border border-gray-300 rounded px-2 py-1 text-sm focus:ring-1 focus:ring-black focus:border-black outline-none"
              >
                <option value="">Select filter...</option>
                {availableFilterKeys.map(k => (
                  <option key={k} value={k}>{k}</option>
                ))}
              </select>
              <span className="text-xs text-gray-500">is</span>
              {getFilterValueOptions(filter.key) ? (
                <select
                  value={filter.value}
                  onChange={e => updateFilter(index, filter.key, e.target.value)}
                  className="bg-white border border-gray-300 rounded px-2 py-1 text-sm focus:ring-1 focus:ring-black focus:border-black outline-none flex-1"
                >
                  <option value="">Select value...</option>
                  {getFilterValueOptions(filter.key)!.map(v => (
                    <option key={v} value={v}>{v}</option>
                  ))}
                </select>
              ) : (
                <input
                  type="text"
                  value={filter.value}
                  onChange={e => updateFilter(index, filter.key, e.target.value)}
                  placeholder={filter.key === 'tags' ? 'comma separated' : 'Enter value...'}
                  className="bg-white border border-gray-300 rounded px-2 py-1 text-sm focus:ring-1 focus:ring-black focus:border-black outline-none flex-1"
                />
              )}
              <button
                type="button"
                onClick={() => removeFilter(index)}
                className="text-gray-400 hover:text-red-500 transition-colors"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={addFilter}
            className="text-xs text-gray-600 hover:text-gray-900 flex items-center gap-1 ml-10 transition-colors"
          >
            <Plus className="w-3 h-3" />
            add filter
          </button>
        </div>

        {/* Group By */}
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <span className="text-sm text-gray-500">grouped by</span>
          <select
            value={definition.group_by}
            onChange={e => updateDefinition({ group_by: e.target.value })}
            className="bg-white border border-gray-300 rounded px-2 py-1 text-sm focus:ring-1 focus:ring-black focus:border-black outline-none"
          >
            {availableGroupByOptions.map(g => (
              <option key={g} value={g}>{g}</option>
            ))}
          </select>
        </div>

        {/* Visualisation — commented out for now
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <span className="text-sm text-gray-500">shown as</span>
          <select
            value={definition.visualisation}
            onChange={e => updateDefinition({ visualisation: e.target.value })}
            className="bg-white border border-gray-300 rounded px-2 py-1 text-sm focus:ring-1 focus:ring-black focus:border-black outline-none"
          >
            {visOptions.map(v => (
              <option key={v} value={v}>{v}</option>
            ))}
          </select>
        </div>
        */}
      </div>

      {/* Complexity & Cost */}
      <div className="flex items-center gap-4 text-sm text-gray-600">
        <span>
          Complexity: <span className="font-semibold text-gray-900">{complexity}</span>
        </span>
        <span className="text-gray-300">|</span>
        <span>
          Est. cost: <span className="font-semibold text-gray-900">{estimatedCostCents}¢</span> per profile load
        </span>
      </div>

      {/* Actions */}
      <div className="flex gap-3 pt-2">
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="px-4 py-2 text-sm font-medium text-gray-700 hover:text-gray-900 transition-colors"
          >
            Cancel
          </button>
        )}
        <button
          type="button"
          onClick={handleSave}
          disabled={justSaved}
          className={`flex items-center gap-2 px-5 py-2.5 text-sm font-medium rounded-lg transition-colors ${
            justSaved
              ? 'bg-green-600 text-white'
              : 'bg-black text-white hover:bg-gray-800'
          }`}
        >
          {justSaved ? <Check className="w-4 h-4" /> : <Save className="w-4 h-4" />}
          {justSaved ? 'Added' : 'Save metric'}
        </button>
      </div>
    </div>
  );
}


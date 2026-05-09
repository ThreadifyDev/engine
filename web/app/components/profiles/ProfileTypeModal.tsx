import { useState, useEffect } from 'react';
import { useNavigate } from '@remix-run/react';
import { X, Settings, Plus, FileJson, LayoutTemplate, Edit2 } from 'lucide-react';
import Alert, { isCreditError } from '~/components/Alert';
import { api, ValidationError } from '~/lib/api';
import type { EntityProfileType, MetricsTemplateResponse, EntityTypeMetric, ParameterDefinition, CustomMetricDefinition } from '~/lib/api';
import { TagInput } from './TagInput';
import SentenceBuilderMetricForm from './SentenceBuilderMetricForm';

interface ProfileTypeModalProps {
  isOpen: boolean;
  onClose: () => void;
  mode: 'create' | 'edit';
  initialData?: EntityProfileType | null;
  metricsTemplates: MetricsTemplateResponse[];
  onRefresh: () => Promise<void>;
  persistedTypes?: string[];
}

export function ProfileTypeModal({
  isOpen,
  onClose,
  mode,
  initialData,
  metricsTemplates,
  onRefresh,
  persistedTypes = [],
}: ProfileTypeModalProps) {
  const navigate = useNavigate();

  const defaultData = { name: '', type: [], description: '', metrics: [] };
  const [formData, setFormData] = useState<Partial<EntityProfileType>>(defaultData);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<{ message: string; details?: Array<{ field: string; message: string }> } | null>(null);
  const [addingMode, setAddingMode] = useState<'none' | 'custom' | 'template'>('none');
  const [customFormKey, setCustomFormKey] = useState(0);
  const [markedForDeletion, setMarkedForDeletion] = useState<string[]>([]);
  const [modifiedMetricIds, setModifiedMetricIds] = useState<string[]>([]);
  const [editingMetricIndex, setEditingMetricIndex] = useState<number | null>(null);

  // Reset form when modal opens/closes or initialData changes
  useEffect(() => {
    if (isOpen) {
      setFormData(mode === 'edit' && initialData ? { ...initialData } : { ...defaultData });
      setError(null);
      setAddingMode('none');
      setMarkedForDeletion([]);
      setModifiedMetricIds([]);
      setEditingMetricIndex(null);
    }
  }, [isOpen, mode, initialData]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (formData.type?.length === 0) {
      setError({ message: 'At least one Type Key is required.' });
      return;
    }

    setError(null);
    try {
      setIsSubmitting(true);

      const payload = {
        name: formData.name || '',
        type: formData.type || [],
        description: formData.description || '',
        metrics: formData.metrics || [],
        ...(mode === 'edit' && markedForDeletion.length > 0 ? { marked_for_deletion: markedForDeletion } : {}),
        ...(mode === 'edit' && modifiedMetricIds.length > 0 ? { modified_metric_ids: modifiedMetricIds } : {})
      };

      if (mode === 'edit' && initialData) {
        await api.updateEntityProfileType(initialData.id, payload);
      } else {
        await api.createEntityProfileType(payload);
      }

      onClose();
      await onRefresh();
    } catch (err: any) {
      if (err instanceof ValidationError) {
        setError({
          message: err.message,
          details: err.details,
        });
      } else {
        setError({
          message: err.message || `Failed to ${mode} profile type`,
        });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const addTemplateMetric = (templateId: string) => {
    setFormData({
      ...formData,
      metrics: [...(formData.metrics || []), { template_id: templateId, parameters: {} }]
    });
  };

  const addCustomMetric = (definition: CustomMetricDefinition) => {
    if (editingMetricIndex !== null) {
      // Update existing metric
      const metrics = formData.metrics || [];
      const newMetrics = [...metrics];
      newMetrics[editingMetricIndex] = { ...newMetrics[editingMetricIndex], custom_definition: definition };
      setFormData({ ...formData, metrics: newMetrics });
      
      // Mark as modified if it has an ID
      if (metrics[editingMetricIndex]?.id) {
        setModifiedMetricIds(prev => {
          if (prev.includes(metrics[editingMetricIndex].id!)) return prev;
          return [...prev, metrics[editingMetricIndex].id!];
        });
      }
      
      setEditingMetricIndex(null);
      setAddingMode('none');
    } else {
      // Add new metric
      setFormData({
        ...formData,
        metrics: [...(formData.metrics || []), { custom_definition: definition }]
      });
      setAddingMode('none');
    }
    setCustomFormKey(k => k + 1);
  };

  const removeMetric = (idx: number) => {
    const metrics = formData.metrics || [];
    const metric = metrics[idx];
    // If metric has an id (persisted), mark it for deletion instead of removing from local state
    if (metric?.id) {
      setMarkedForDeletion(prev => [...prev, metric.id!]);
    }
    const newMetrics = [...metrics];
    newMetrics.splice(idx, 1);
    setFormData({ ...formData, metrics: newMetrics });
  };

  const updateMetric = (idx: number, updated: EntityTypeMetric) => {
    const metrics = formData.metrics || [];
    const original = metrics[idx];
    const newMetrics = [...metrics];
    newMetrics[idx] = updated;
    setFormData({ ...formData, metrics: newMetrics });

    // If editing an existing metric, mark it as modified
    if (mode === 'edit' && original?.id && original.id === updated.id) {
      setModifiedMetricIds(prev => {
        if (prev.includes(original.id!)) return prev;
        return [...prev, original.id!];
      });
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4 backdrop-blur-sm">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-5xl overflow-hidden flex flex-col max-h-[92vh]">
        <div className="px-6 py-4 border-b border-gray-200 flex justify-between items-center shrink-0">
          <h2 className="text-lg font-bold text-gray-900">
            {mode === 'edit' ? 'Edit Profile Type' : 'Create Profile Type'}
          </h2>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="flex flex-col flex-1 overflow-hidden">
          <div className="flex-1 overflow-y-auto">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-0">
              {/* Left column — Profile Info */}
              <div className="p-6 border-b md:border-b-0 md:border-r border-gray-200 space-y-5">
                {error && (
                  <Alert
                    type="error"
                    message={error.message}
                    details={error.details}
                    className="mb-4"
                    action={
                      isCreditError(error.message)
                        ? { label: 'Go to Billing', onClick: () => navigate('/u/settings?tab=billing'), variant: 'primary' }
                        : undefined
                    }
                  />
                )}
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
                  <input
                    type="text"
                    required
                    value={formData.name || ''}
                    onChange={e => setFormData({...formData, name: e.target.value})}
                    placeholder="e.g. Courier Profile"
                    className="w-full px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Type Key(s)</label>
                  <TagInput
                    tags={formData.type || []}
                    persistedTags={mode === 'edit' ? persistedTypes : []}
                    onChange={tags => setFormData({...formData, type: tags})}
                    placeholder={mode === 'create' ? "e.g. courier_id" : "Add more keys..."}
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    {mode === 'edit'
                      ? "Already persisted keys cannot be removed. You can add new keys."
                      : "Press Enter, comma, or space to add a key. Keys must match how entities are referenced in thread steps."}
                  </p>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
                  <textarea
                    rows={4}
                    maxLength={255}
                    value={formData.description || ''}
                    onChange={e => setFormData({...formData, description: e.target.value})}
                    placeholder="Optional description (max 255 chars)..."
                    className="w-full px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none resize-none"
                  />
                  <div className="text-right text-[10px] text-gray-400 mt-1">
                    {(formData.description || '').length}/255
                  </div>
                </div>
              </div>

              {/* Right column — Metrics */}
              <div className="p-6 flex flex-col">
                <h3 className="text-sm font-semibold text-gray-900 mb-3">Metrics Configuration</h3>

                <div className="flex-1 space-y-3 min-h-[120px]">
                  {formData.metrics && formData.metrics.length > 0 ? (
                    formData.metrics.map((m, idx) => {
                      if (m.custom_definition) {
                        return (
                          <CustomMetricCard
                            key={`custom-${idx}`}
                            metric={m}
                            onRemove={() => removeMetric(idx)}
                            onEdit={() => {
                              setAddingMode('custom');
                              setCustomFormKey(k => k + 1);
                              setEditingMetricIndex(idx);
                            }}
                          />
                        );
                      }
                      const template = metricsTemplates.find(t => t.id === m.template_id);
                      if (!template) return null;
                      return (
                        <MetricSelectionCard
                          key={`${m.template_id}-${idx}`}
                          template={template}
                          metric={m}
                          onChange={(updatedMetric) => updateMetric(idx, updatedMetric)}
                          onRemove={() => removeMetric(idx)}
                        />
                      );
                    })
                  ) : (
                    <div className="bg-gray-50 border border-dashed border-gray-300 rounded-lg p-6 text-center">
                      <p className="text-sm text-gray-500">No metrics configured yet.</p>
                      <p className="text-xs text-gray-400 mt-1">Add custom metrics or choose from templates.</p>
                    </div>
                  )}
                </div>

                {/* Inline metric addition controls */}
                <div className="mt-4 pt-4 border-t border-gray-100">
                  {addingMode === 'none' && (
                    <div className="flex gap-2">
                      <button
                        type="button"
                        onClick={() => setAddingMode('custom')}
                        className="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 bg-gray-900 text-white text-sm font-medium rounded-lg hover:bg-gray-800 transition-colors"
                      >
                        <Plus className="w-4 h-4" />
                        Custom Metric
                      </button>
                      <button
                        type="button"
                        onClick={() => setAddingMode('template')}
                        className="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 bg-white border border-gray-300 text-gray-700 text-sm font-medium rounded-lg hover:bg-gray-50 transition-colors"
                      >
                        <LayoutTemplate className="w-4 h-4" />
                        From Template
                      </button>
                    </div>
                  )}

                  {addingMode === 'custom' && (
                    <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
                      <div className="flex items-center justify-between mb-3">
                        <h4 className="text-sm font-semibold text-gray-900">
                          {editingMetricIndex !== null ? 'Edit Custom Metric' : 'New Custom Metric'}
                        </h4>
                        <button
                          type="button"
                          onClick={() => {
                            setAddingMode('none');
                            setEditingMetricIndex(null);
                          }}
                          className="text-gray-400 hover:text-gray-600"
                        >
                          <X className="w-4 h-4" />
                        </button>
                      </div>
                      <SentenceBuilderMetricForm
                        key={customFormKey}
                        initialData={editingMetricIndex !== null ? formData.metrics?.[editingMetricIndex]?.custom_definition : undefined}
                        onSubmit={addCustomMetric}
                        onCancel={() => {
                          setAddingMode('none');
                          setEditingMetricIndex(null);
                        }}
                      />
                    </div>
                  )}

                  {addingMode === 'template' && (
                    <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
                      <div className="flex items-center justify-between mb-3">
                        <h4 className="text-sm font-semibold text-gray-900">Choose a Template</h4>
                        <button
                          type="button"
                          onClick={() => setAddingMode('none')}
                          className="text-gray-400 hover:text-gray-600"
                        >
                          <X className="w-4 h-4" />
                        </button>
                      </div>
                      {metricsTemplates.length === 0 ? (
                        <p className="text-sm text-gray-500 italic">No templates available.</p>
                      ) : (
                        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                          {metricsTemplates.map((t) => (
                            <button
                              key={t.id}
                              type="button"
                              onClick={() => addTemplateMetric(t.id)}
                              className="text-left px-3 py-2.5 text-sm text-gray-700 bg-white border border-gray-200 rounded-lg hover:border-gray-900 hover:shadow-sm transition-all"
                            >
                              <span className="font-medium">{t.metrics_name}</span>
                              {(t.parameter_definitions || []).length > 0 && (
                                <span className="block text-xs text-gray-400 mt-0.5">
                                  {(t.parameter_definitions || []).length} parameter{(t.parameter_definitions || []).length !== 1 ? 's' : ''}
                                </span>
                              )}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
          <div className="px-6 py-4 bg-gray-50 border-t border-gray-200 flex justify-end gap-3 shrink-0">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm font-medium text-gray-700 hover:text-gray-900 transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="px-5 py-2 bg-black text-white text-sm font-medium rounded-lg hover:bg-gray-800 disabled:opacity-50 transition-colors flex items-center"
            >
              {isSubmitting ? (
                <>
                  <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-white" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                  </svg>
                  Saving...
                </>
              ) : mode === 'edit' ? 'Save Changes' : 'Create Profile Type'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

// Sub-component for Metric Configuration
function MetricSelectionCard({
  template,
  metric,
  onChange,
  onRemove
}: {
  template: MetricsTemplateResponse;
  metric: EntityTypeMetric;
  onChange: (m: EntityTypeMetric) => void;
  onRemove: () => void;
}) {
  const handleParamChange = (param: string, value: any) => {
    onChange({
      ...metric,
      parameters: { ...metric.parameters, [param]: value }
    });
  };

  const renderParamInput = (def: ParameterDefinition) => {
    const val = metric.parameters?.[def.name];

    switch (def.type) {
      case 'enum':
        return (
          <select
            value={val || ''}
            onChange={(e) => handleParamChange(def.name, e.target.value)}
            className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:border-black focus:ring-1 focus:ring-black outline-none bg-white"
          >
            <option value="" disabled>Select {def.name}...</option>
            {(def.values || []).map((v: string) => (
              <option key={v} value={v}>{v}</option>
            ))}
          </select>
        );

      case 'number':
        return (
          <input
            type="number"
            value={val || ''}
            onChange={(e) => handleParamChange(def.name, e.target.value)}
            className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:border-black focus:ring-1 focus:ring-black outline-none bg-white"
            placeholder={def.description || `Enter ${def.name}...`}
          />
        );

      case 'boolean':
        return (
          <label className="flex items-center gap-2 cursor-pointer select-none">
            <div className="relative">
              <input
                type="checkbox"
                checked={val === true || val === 'true'}
                onChange={(e) => handleParamChange(def.name, e.target.checked)}
                className="sr-only peer"
              />
              <div className="w-9 h-5 bg-gray-300 peer-focus:outline-none peer-focus:ring-1 peer-focus:ring-black rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-black" />
            </div>
            <span className="text-sm text-gray-700">{def.description || def.name}</span>
          </label>
        );

      case 'string':
      default:
        return (
          <input
            type="text"
            value={val || ''}
            onChange={(e) => handleParamChange(def.name, e.target.value)}
            className="w-full px-2 py-1.5 text-sm border border-gray-300 rounded focus:border-black focus:ring-1 focus:ring-black outline-none bg-white"
            placeholder={def.description || `Enter ${def.name}...`}
          />
        );
    }
  };

  return (
    <div className="bg-gray-50 border border-gray-200 rounded-lg p-4 relative group">
      <button
        type="button"
        onClick={onRemove}
        className="absolute top-3 right-3 text-gray-400 hover:text-red-500 opacity-0 group-hover:opacity-100 transition-opacity"
      >
        <X className="w-4 h-4" />
      </button>

      <div className="flex items-center gap-2 mb-3">
        <Settings className="w-4 h-4 text-gray-500" />
        <h4 className="text-sm font-semibold text-gray-900">{template.metrics_name}</h4>
      </div>

      {(template.parameter_definitions || []).length === 0 ? (
        <p className="text-xs text-gray-500 italic">No configuration required for this metric.</p>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {(template.parameter_definitions || []).map((def) => (
            <div key={def.name} className="flex flex-col gap-1">
              <label className="text-xs font-mono text-gray-600">@{def.name}</label>
              {renderParamInput(def)}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// Card displaying a custom metric definition
function CustomMetricCard({
  metric,
  onRemove,
  onEdit,
}: {
  metric: EntityTypeMetric;
  onRemove: () => void;
  onEdit: () => void;
}) {
  const def = metric.custom_definition!;
  const targetLabel = def.target === 'step' ? `Step${def.step_name ? ` (${def.step_name})` : ''}` : 'Thread';

  return (
    <div className="bg-gray-50 border border-gray-200 rounded-lg p-4 relative group">
      <div className="absolute top-3 right-3 flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
        <button
          type="button"
          onClick={onEdit}
          className="p-1 text-gray-400 hover:text-black hover:bg-gray-100 rounded transition-colors"
          title="Edit metric"
        >
          <Edit2 className="w-3.5 h-3.5" />
        </button>
        <button
          type="button"
          onClick={onRemove}
          className="p-1 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded transition-colors"
          title="Remove metric"
        >
          <X className="w-4 h-4" />
        </button>
      </div>

      <div className="flex items-center gap-2 mb-2">
        <FileJson className="w-4 h-4 text-gray-500" />
        <h4 className="text-sm font-semibold text-gray-900">{def.name}</h4>
        <span className="text-[10px] font-mono text-gray-500 bg-gray-200 px-1.5 py-0.5 rounded">
          Custom
        </span>
      </div>

      <div className="flex flex-wrap gap-2 text-xs text-gray-600">
        <span className="bg-white border border-gray-200 px-2 py-0.5 rounded">
          Target: {targetLabel}
        </span>
        {def.operation && (
          <span className="bg-white border border-gray-200 px-2 py-0.5 rounded">
            {def.operation}({def.field})
          </span>
        )}
        {def.group_by && def.group_by !== 'none' && (
          <span className="bg-white border border-gray-200 px-2 py-0.5 rounded">
            Group: {def.group_by}
          </span>
        )}
        {def.visualisation && (
          <span className="bg-white border border-gray-200 px-2 py-0.5 rounded">
            {def.visualisation}
          </span>
        )}
      </div>
    </div>
  );
}

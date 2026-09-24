import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router';
import type { EntityProfileType, MetricsTemplateResponse, EntityTypeMetric } from '~/lib/api';
import { Database, Plus, Edit2, Trash2, X, ArrowRight, Settings, Copy } from 'lucide-react';
import Alert, { isCreditError } from '~/components/Alert';
import { api, ValidationError } from '~/lib/api';
import { ProfileTypeModal } from './ProfileTypeModal';

interface ProfileTypesTabProps {
  profileTypes: EntityProfileType[];
  isLoading: boolean;
  error: string | null;
  onRefresh: () => Promise<void>;
}

export default function ProfileTypesTab({ profileTypes, isLoading, error, onRefresh }: ProfileTypesTabProps) {
  const navigate = useNavigate();
  const [metricsTemplates, setMetricsTemplates] = useState<MetricsTemplateResponse[]>([]);

  useEffect(() => {
    api.listMetricsTemplates()
      .then(res => setMetricsTemplates(res.data))
      .catch(console.error);
  }, []);

  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [editData, setEditData] = useState<EntityProfileType | null>(null);

  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteData, setDeleteData] = useState<EntityProfileType | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const handleDelete = async () => {
    if (!deleteData) return;
    setDeleteError(null);
    try {
      setIsDeleting(true);
      await api.archiveEntityProfileType(deleteData.slug);
      setDeleteData(null);
      await onRefresh();
    } catch (err: any) {
      setDeleteError(err.message || 'Failed to archive profile type');
    } finally {
      setIsDeleting(false);
    }
  };

  const handleDuplicate = (pt: EntityProfileType) => {
    const clone: Partial<EntityProfileType> = JSON.parse(JSON.stringify(pt));
    delete clone.id;
    delete clone.slug;
    // @ts-ignore
    delete clone.created_at;
    // @ts-ignore
    delete clone.updated_at;
    // @ts-ignore
    delete clone.company_id;
    
    clone.name = `Copy of ${clone.name}`;
    if (clone.metrics) {
      clone.metrics = clone.metrics.map(m => {
        const mClone = { ...m };
        delete mClone.id;
        return mClone;
      });
    }
    
    setEditData(clone as EntityProfileType);
    setIsCreateModalOpen(true);
  };

  const persistedTypes = editData?.type || [];

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-4 rounded-2xl border border-stone-200 bg-white px-6 py-5 shadow-sm">
        <div>
          <h2 className="text-lg font-semibold tracking-tight text-stone-900">Profile types <span className="ml-2 rounded-full bg-stone-100 px-2.5 py-1 text-xs font-medium text-stone-600">{profileTypes.length}</span></h2>
          <p className="mt-1 text-sm text-stone-500">Define the entities and metrics tracked across your threads.</p>
        </div>
        <button
          onClick={() => setIsCreateModalOpen(true)}
          className="inline-flex items-center gap-2 rounded-lg bg-stone-900 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-stone-700"
        >
          <Plus className="h-4 w-4" /> New profile type
        </button>
      </div>

      {/* Content */}
      <div className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm">
        {isLoading ? (
          <div className="flex flex-col items-center p-12 text-center text-stone-500">
            <div className="w-8 h-8 border-2 border-gray-300 border-t-black rounded-full animate-spin mb-3"></div>
            Loading profile types...
          </div>
        ) : error ? (
          <div className="p-8 text-center text-red-600">
            {error}
          </div>
        ) : profileTypes.length === 0 ? (
          <div className="flex flex-col items-center p-12 text-center">
            <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-emerald-50">
              <Database className="h-7 w-7 text-emerald-700" />
            </div>
            <h3 className="mb-2 text-lg font-semibold text-stone-900">No profile types yet</h3>
            <p className="mx-auto mb-6 max-w-md text-sm text-stone-500">
              Create a profile type to start tracking entity metrics (like customers, couriers, or vendors) across your threads.
            </p>
            <button
              onClick={() => setIsCreateModalOpen(true)}
              className="rounded-lg bg-stone-900 px-4 py-2.5 text-sm font-medium text-white transition hover:bg-stone-700"
            >
              Create Your First Profile Type
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 p-5 md:grid-cols-2 xl:grid-cols-3">
            {profileTypes.map((pt) => (
              <div 
                key={pt.id} 
                className="group flex min-w-0 flex-col rounded-xl border border-stone-200 bg-white p-5 transition-all duration-200 hover:border-emerald-200 hover:shadow-md"
              >
                <div className="flex justify-between items-start mb-3">
                  <h3 className="min-w-0 break-words text-base font-semibold text-stone-900">{pt.name}</h3>
                  <div className="flex shrink-0 gap-1">
                    <button
                      title="Duplicate"
                      onClick={() => handleDuplicate(pt)}
                      className="rounded-md p-1.5 text-stone-400 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500"
                    >
                      <Copy className="w-3.5 h-3.5" />
                    </button>
                    <button
                      title="Edit"
                      onClick={() => navigate(`/u/profile-views/${encodeURIComponent(pt.name)}?tab=data`)}
                      className="rounded-md p-1.5 text-stone-400 transition-colors hover:bg-stone-100 hover:text-stone-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500"
                    >
                      <Edit2 className="w-3.5 h-3.5" />
                    </button>
                    <button
                      title="Archive"
                      onClick={() => setDeleteData(pt)}
                      className="rounded-md p-1.5 text-stone-400 transition-colors hover:bg-red-50 hover:text-red-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                <div className="flex flex-wrap gap-1 mb-3">
                  {pt.type.map(t => (
                    <code key={t} className="rounded-md border border-emerald-100 bg-emerald-50 px-2 py-0.5 font-mono text-[11px] text-emerald-700">
                      {t}
                    </code>
                  ))}
                </div>
                <p className="line-clamp-2 flex-1 text-sm leading-6 text-stone-600">
                  {pt.description || <span className="text-gray-400 italic">No description</span>}
                </p>

                {pt.metrics && pt.metrics.length > 0 && (
                  <div className="mt-3 flex flex-wrap gap-1">
                    {pt.metrics.map((m, i) => {
                      if (m.custom_definition) {
                        return (
                          <span key={i} className="inline-flex items-center gap-1 text-[10px] font-medium bg-purple-50 text-purple-600 border border-purple-100 px-1.5 py-0.5 rounded group cursor-pointer hover:bg-purple-100 transition-colors"
                            onClick={() => navigate(`/u/profile-views/${encodeURIComponent(pt.name)}?tab=data`)}
                            title="Click to edit this metric"
                          >
                            <Settings className="w-2.5 h-2.5" />
                            {m.custom_definition.name || 'Custom Metric'}
                          </span>
                        );
                      }
                      const tmpl = metricsTemplates.find(t => t.id === m.template_id);
                      return (
                        <span key={i} className="inline-flex items-center gap-1 text-[10px] font-medium bg-blue-50 text-blue-600 border border-blue-100 px-1.5 py-0.5 rounded">
                          <Settings className="w-2.5 h-2.5" />
                          {tmpl?.metrics_name ?? m.template_id}
                        </span>
                      );
                    })}
                  </div>
                )}

                <div className="mt-5 flex items-center justify-between gap-3 border-t border-stone-100 pt-4 text-xs text-stone-500">
                  <span>{pt.updated_at ? `Updated ${new Date(pt.updated_at).toLocaleDateString()}` : '—'}</span>
                  <button type="button"
                    className="inline-flex shrink-0 items-center gap-1 font-medium text-emerald-700 hover:text-emerald-900"
                    onClick={() => navigate(`/u/profiles/${encodeURIComponent(pt.name || '')}`)}
                  >
                    View profiles <ArrowRight className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <ProfileTypeModal
        isOpen={isCreateModalOpen}
        mode="create"
        initialData={editData}
        onClose={() => {
          setIsCreateModalOpen(false);
          setEditData(null);
        }}
        metricsTemplates={metricsTemplates}
        onRefresh={onRefresh}
      />

      <ProfileTypeModal
        isOpen={isEditModalOpen}
        mode="edit"
        initialData={editData}
        onClose={() => {
          setIsEditModalOpen(false);
          setEditData(null);
        }}
        metricsTemplates={metricsTemplates}
        onRefresh={onRefresh}
        persistedTypes={persistedTypes}
      />

      {/* Delete/Archive Confirmation Modal */}
      {deleteData && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4 backdrop-blur-sm">
          <div className="w-full max-w-sm rounded-2xl border border-stone-200 bg-white p-6 text-center shadow-xl">
            <div className="w-12 h-12 rounded-full bg-red-100 mx-auto flex items-center justify-center mb-4">
              <Trash2 className="w-6 h-6 text-red-600" />
            </div>
            <h3 className="text-lg font-bold text-gray-900 mb-2">Archive Profile Type?</h3>
            <p className="text-sm text-gray-500 mb-6">
              Are you sure you want to archive <strong>{deleteData.name}</strong>? This will stop tracking new profiles of this type, but existing data will be preserved.
            </p>
            {deleteError && (
              <div className="mb-4 p-3 bg-red-50 text-red-600 text-sm rounded border border-red-100 text-left">
                {deleteError}
              </div>
            )}
            <div className="flex gap-3 justify-center">
              <button
                onClick={() => { setDeleteData(null); setDeleteError(null); }}
                className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded hover:bg-gray-50 transition-colors"
                disabled={isDeleting}
              >
                Cancel
              </button>
              <button
                onClick={handleDelete}
                disabled={isDeleting}
                className="px-4 py-2 text-sm font-medium text-white bg-red-600 rounded hover:bg-red-700 disabled:opacity-50 transition-colors flex items-center gap-2"
              >
                {isDeleting && <div className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
                Archive Profile Type
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

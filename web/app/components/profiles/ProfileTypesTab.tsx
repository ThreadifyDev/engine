import { useState, useEffect } from 'react';
import { useNavigate } from '@remix-run/react';
import type { EntityProfileType, MetricsTemplateResponse, EntityTypeMetric } from '~/lib/api';
import { Database, Plus, Edit2, Trash2, X, ArrowRight, Settings } from 'lucide-react';
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
      await api.archiveEntityProfileType(deleteData.id);
      setDeleteData(null);
      await onRefresh();
    } catch (err: any) {
      setDeleteError(err.message || 'Failed to archive profile type');
    } finally {
      setIsDeleting(false);
    }
  };

  const persistedTypes = editData?.type || [];

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center border-b border-gray-200 pb-5">
        <div>
          <h2 className="text-xl font-bold text-gray-900">Profile Types</h2>
          <p className="text-sm text-gray-500 mt-1">Configure the kinds of entities you want to track across your threads.</p>
        </div>
        <button
          onClick={() => setIsCreateModalOpen(true)}
          className="flex items-center gap-2 bg-black text-white px-4 py-2 rounded-lg text-sm font-medium hover:bg-gray-800 transition-colors shadow-sm"
        >
          <Plus className="w-4 h-4" /> Create Profile Type
        </button>
      </div>

      {/* Content */}
      <div className="bg-white rounded-lg border border-gray-200">
        {isLoading ? (
          <div className="p-8 text-center text-gray-500 flex flex-col items-center">
            <div className="w-8 h-8 border-2 border-gray-300 border-t-black rounded-full animate-spin mb-3"></div>
            Loading profile types...
          </div>
        ) : error ? (
          <div className="p-8 text-center text-red-500">
            {error}
          </div>
        ) : profileTypes.length === 0 ? (
          <div className="p-12 text-center flex flex-col items-center">
            <div className="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mb-4">
              <Database className="w-8 h-8 text-gray-400" />
            </div>
            <h3 className="text-lg font-medium text-gray-900 mb-2">No Profile Types Yet</h3>
            <p className="text-sm text-gray-500 max-w-md mx-auto mb-6">
              Create a profile type to start tracking entity metrics (like customers, couriers, or vendors) across your threads.
            </p>
            <button
              onClick={() => setIsCreateModalOpen(true)}
              className="bg-white text-black border border-gray-300 px-4 py-2 rounded-lg text-sm font-medium hover:bg-gray-50 transition-colors shadow-sm"
            >
              Create Your First Profile Type
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 p-6 bg-gray-50/50">
            {profileTypes.map((pt) => (
              <div 
                key={pt.id} 
                className="bg-white rounded-xl border border-gray-200 p-5 hover:border-black/20 hover:shadow-md transition-all duration-200 group flex flex-col"
              >
                <div className="flex justify-between items-start mb-3">
                  <h3 className="text-base font-semibold text-gray-900 mb-1">{pt.name}</h3>
                  <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                    <button
                      title="Edit"
                      onClick={() => { setEditData(pt); setIsEditModalOpen(true); }}
                      className="p-1.5 text-gray-400 hover:text-black hover:bg-gray-100 rounded transition-colors"
                    >
                      <Edit2 className="w-3.5 h-3.5" />
                    </button>
                    <button
                      title="Archive"
                      onClick={() => setDeleteData(pt)}
                      className="p-1.5 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded transition-colors"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                <div className="flex flex-wrap gap-1 mb-3">
                  {pt.type.map(t => (
                    <code key={t} className="text-[10px] font-mono bg-gray-50 text-gray-500 px-1.5 py-0.5 border border-gray-200 rounded">
                      {t}
                    </code>
                  ))}
                </div>
                <p className="text-sm text-gray-600 flex-1 line-clamp-2">
                  {pt.description || <span className="text-gray-400 italic">No description</span>}
                </p>

                {pt.metrics && pt.metrics.length > 0 && (
                  <div className="mt-3 flex flex-wrap gap-1">
                    {pt.metrics.map((m, i) => {
                      if (m.custom_definition) {
                        return (
                          <span key={i} className="inline-flex items-center gap-1 text-[10px] font-medium bg-purple-50 text-purple-600 border border-purple-100 px-1.5 py-0.5 rounded group cursor-pointer hover:bg-purple-100 transition-colors"
                            onClick={() => { setEditData(pt); setIsEditModalOpen(true); }}
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

                <div className="mt-4 pt-4 border-t border-gray-100 flex items-center justify-between text-xs text-gray-500">
                  <span>{pt.updated_at ? `Updated ${new Date(pt.updated_at).toLocaleDateString()}` : '—'}</span>
                  <span 
                    className="flex items-center gap-1 text-gray-700 group-hover:text-black font-medium cursor-pointer"
                    onClick={() => navigate(`/u/profiles/${encodeURIComponent(pt.name || '')}`)}
                  >
                    View profiles <ArrowRight className="w-3.5 h-3.5" />
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <ProfileTypeModal
        isOpen={isCreateModalOpen}
        mode="create"
        onClose={() => setIsCreateModalOpen(false)}
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
          <div className="bg-white rounded-xl shadow-xl max-w-sm w-full p-6 text-center">
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

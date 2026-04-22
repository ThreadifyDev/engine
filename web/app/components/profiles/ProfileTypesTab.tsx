import { useState } from 'react';
import { useNavigate } from '@remix-run/react';
import type { EntityProfileType } from '~/lib/api';
import { Database, Plus, Edit2, Trash2, X, ArrowRight } from 'lucide-react';
import Alert, { isCreditError } from '~/components/Alert';
import { api, ValidationError } from '~/lib/api';

interface ProfileTypesTabProps {
  profileTypes: EntityProfileType[];
  isLoading: boolean;
  error: string | null;
  onRefresh: () => Promise<void>;
}

export default function ProfileTypesTab({ profileTypes, isLoading, error, onRefresh }: ProfileTypesTabProps) {
  const navigate = useNavigate();
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [createData, setCreateData] = useState({ name: '', type: '', description: '' });
  const [createError, setCreateError] = useState<{ message: string; details?: Array<{ field: string; message: string }> } | null>(null);

  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editData, setEditData] = useState<EntityProfileType | null>(null);

  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteData, setDeleteData] = useState<EntityProfileType | null>(null);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    setCreateError(null);
    try {
      setIsCreating(true);
      await api.createEntityProfileType(createData);
      setCreateData({ name: '', type: '', description: '' });
      setIsCreateModalOpen(false);
      await onRefresh();
    } catch (err: any) {
      if (err instanceof ValidationError) {
        setCreateError({
          message: err.message,
          details: err.details,
        });
      } else {
        setCreateError({
          message: err.message || 'Failed to create profile type',
        });
      }
    } finally {
      setIsCreating(false);
    }
  };

  const handleEdit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editData) return;
    try {
      setIsEditing(true);
      await api.updateEntityProfileType(editData.id, { name: editData.name, description: editData.description });
      setIsEditModalOpen(false);
      setEditData(null);
      await onRefresh();
    } catch (err: any) {
      alert(err.message || 'Failed to update profile type');
    } finally {
      setIsEditing(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteData) return;
    try {
      setIsDeleting(true);
      await api.archiveEntityProfileType(deleteData.id);
      setIsDeleteOpen(false);
      setDeleteData(null);
      await onRefresh();
    } catch (err: any) {
      alert(err.message || 'Failed to delete profile type');
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-lg font-semibold text-gray-900">Profile Types</h2>
          <p className="text-sm text-gray-600 mt-1">
            Define the schemas for your tracked entities
          </p>
        </div>
        <button
          onClick={() => setIsCreateModalOpen(true)}
          className="px-4 py-2 bg-gray-900 text-white text-sm font-medium hover:bg-gray-800 transition-colors rounded flex items-center gap-2"
        >
          <Plus className="w-4 h-4" /> Create Profile Type
        </button>
      </div>

      {isLoading ? (
        <div className="bg-white border border-gray-200 rounded-lg p-8 text-center text-gray-500">Loading profile types...</div>
      ) : error ? (
        <div className="bg-white border border-gray-200 rounded-lg p-8 text-center text-red-500">{error}</div>
      ) : profileTypes.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-lg p-12 text-center flex flex-col items-center">
          <Database className="w-12 h-12 text-gray-300 mb-3" />
          <h3 className="text-lg font-medium text-gray-900">No Profile Types Found</h3>
          <p className="text-gray-500 mt-1">Create your first Entity Profile Type to start tracking dimensions across processes.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {profileTypes.map((pt) => (
            <div
              key={pt.id}
              role="button"
              tabIndex={0}
              onClick={() => navigate(`/u/profiles/${encodeURIComponent(pt.type)}`)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  navigate(`/u/profiles/${encodeURIComponent(pt.type)}`);
                }
              }}
              className="group relative bg-white border border-gray-200 rounded-lg p-5 shadow-sm hover:shadow-md hover:border-gray-300 transition-all cursor-pointer flex flex-col"
            >
              <div className="flex items-start justify-between mb-3">
                <div className="w-10 h-10 rounded-lg bg-gray-100 flex items-center justify-center">
                  <Database className="w-5 h-5 text-gray-700" />
                </div>
                <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                  <button
                    onClick={(e) => { e.stopPropagation(); setEditData(pt); setIsEditModalOpen(true); }}
                    className="p-1.5 text-gray-400 hover:text-black transition-colors rounded hover:bg-gray-100"
                    title="Edit"
                  >
                    <Edit2 className="w-4 h-4" />
                  </button>
                  <button
                    onClick={(e) => { e.stopPropagation(); setDeleteData(pt); setIsDeleteOpen(true); }}
                    className="p-1.5 text-gray-400 hover:text-red-600 transition-colors rounded hover:bg-red-50"
                    title="Delete"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>

              <h3 className="text-base font-semibold text-gray-900 mb-1">{pt.name}</h3>
              <code className="text-xs font-mono text-gray-500 mb-3 inline-block">{pt.type}</code>
              <p className="text-sm text-gray-600 flex-1 line-clamp-2">
                {pt.description || <span className="text-gray-400 italic">No description</span>}
              </p>

              <div className="mt-4 pt-4 border-t border-gray-100 flex items-center justify-between text-xs text-gray-500">
                <span>{pt.updated_at ? `Updated ${new Date(pt.updated_at).toLocaleDateString()}` : '—'}</span>
                <span className="flex items-center gap-1 text-gray-700 group-hover:text-black font-medium">
                  View profiles <ArrowRight className="w-3.5 h-3.5" />
                </span>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create Modal */}
      {isCreateModalOpen && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4 backdrop-blur-sm">
          <div className="bg-white rounded-xl shadow-xl w-full max-w-md overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
              <h2 className="text-lg font-bold text-gray-900">Create Profile Type</h2>
              <button 
                onClick={() => {
                  setIsCreateModalOpen(false);
                  setCreateError(null);
                }}
                className="text-gray-400 hover:text-gray-600"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            <form onSubmit={handleCreate} className="p-6">
              {createError && (
                <Alert
                  type="error"
                  message={createError.message}
                  details={createError.details}
                  className="mb-4"
                  action={
                    isCreditError(createError.message)
                      ? { label: 'Go to Billing', onClick: () => navigate('/u/settings?tab=billing'), variant: 'primary' }
                      : undefined
                  }
                />
              )}
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
                  <input
                    type="text"
                    required
                    value={createData.name}
                    onChange={e => setCreateData({...createData, name: e.target.value})}
                    placeholder="e.g. Courier Profile"
                    className="w-full px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Type Key</label>
                  <input
                    type="text"
                    required
                    value={createData.type}
                    onChange={e => setCreateData({...createData, type: e.target.value})}
                    placeholder="e.g. courier_id (must match ref key in steps)"
                    className="w-full px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none font-mono text-sm"
                  />
                  <p className="text-xs text-gray-500 mt-1">This key must exactly match how this entity is referenced in thread steps.</p>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
                  <textarea
                    rows={3}
                    value={createData.description}
                    onChange={e => setCreateData({...createData, description: e.target.value})}
                    placeholder="Optional description..."
                    className="w-full px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none resize-none"
                  />
                </div>
              </div>
              <div className="mt-8 flex justify-end gap-3">
                <button
                  type="button"
                  onClick={() => setIsCreateModalOpen(false)}
                  className="px-4 py-2 border border-gray-300 text-gray-700 hover:bg-gray-50 rounded transition-colors text-sm font-medium"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isCreating}
                  className="px-4 py-2 bg-black text-white hover:bg-gray-800 rounded transition-colors text-sm font-medium disabled:opacity-50"
                >
                  {isCreating ? 'Creating...' : 'Create Type'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Modal */}
      {isEditModalOpen && editData && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4 backdrop-blur-sm">
          <div className="bg-white rounded-xl shadow-xl w-full max-w-md overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
              <h2 className="text-lg font-bold text-gray-900">Edit Profile Type</h2>
              <button 
                onClick={() => { setIsEditModalOpen(false); setEditData(null); }}
                className="text-gray-400 hover:text-gray-600"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            <form onSubmit={handleEdit} className="p-6">
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
                  <input
                    type="text"
                    required
                    value={editData.name}
                    onChange={e => setEditData({...editData, name: e.target.value})}
                    className="w-full px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Type Key</label>
                  <input
                    type="text"
                    disabled
                    value={editData.type}
                    className="w-full px-3 py-2 border border-gray-200 bg-gray-50 text-gray-500 rounded outline-none font-mono text-sm cursor-not-allowed"
                  />
                  <p className="text-xs text-gray-500 mt-1">The type key cannot be changed once created.</p>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
                  <textarea
                    rows={3}
                    value={editData.description || ''}
                    onChange={e => setEditData({...editData, description: e.target.value})}
                    className="w-full px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none resize-none"
                  />
                </div>
              </div>
              <div className="mt-8 flex justify-end gap-3">
                <button
                  type="button"
                  onClick={() => { setIsEditModalOpen(false); setEditData(null); }}
                  className="px-4 py-2 border border-gray-300 text-gray-700 hover:bg-gray-50 rounded transition-colors text-sm font-medium"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isEditing}
                  className="px-4 py-2 bg-black text-white hover:bg-gray-800 rounded transition-colors text-sm font-medium disabled:opacity-50"
                >
                  {isEditing ? 'Saving...' : 'Save Changes'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {isDeleteOpen && deleteData && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4 backdrop-blur-sm">
          <div className="bg-white rounded-xl shadow-xl w-full max-w-sm overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-200">
              <h2 className="text-lg font-bold text-red-600">Delete Profile Type</h2>
            </div>
            <div className="p-6">
              <p className="text-gray-700 mb-6">
                Are you sure you want to delete the <strong>{deleteData.name}</strong> profile type? 
                This action cannot be undone and will prevent future threads from updating this profile.
              </p>
              <div className="flex justify-end gap-3">
                <button
                  type="button"
                  onClick={() => { setIsDeleteOpen(false); setDeleteData(null); }}
                  className="px-4 py-2 border border-gray-300 text-gray-700 hover:bg-gray-50 rounded transition-colors text-sm font-medium"
                >
                  Cancel
                </button>
                <button
                  onClick={confirmDelete}
                  disabled={isDeleting}
                  className="px-4 py-2 bg-red-600 text-white hover:bg-red-700 rounded transition-colors text-sm font-medium disabled:opacity-50"
                >
                  {isDeleting ? 'Deleting...' : 'Delete'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

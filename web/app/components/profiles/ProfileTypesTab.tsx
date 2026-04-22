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
  const [createData, setCreateData] = useState<{ name: string; type: string[]; description: string }>({ name: '', type: [], description: '' });
  const [createError, setCreateError] = useState<{ message: string; details?: Array<{ field: string; message: string }> } | null>(null);

  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editData, setEditData] = useState<EntityProfileType | null>(null);
  const [editError, setEditError] = useState<{ message: string; details?: Array<{ field: string; message: string }> } | null>(null);
  const [persistedTypes, setPersistedTypes] = useState<string[]>([]);

  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteData, setDeleteData] = useState<EntityProfileType | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (createData.type.length === 0) {
      setCreateError({ message: 'At least one Type Key is required.' });
      return;
    }
    setCreateError(null);
    try {
      setIsCreating(true);
      await api.createEntityProfileType(createData);
      setCreateData({ name: '', type: [], description: '' });
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
    setEditError(null);
    try {
      setIsEditing(true);
      await api.updateEntityProfileType(editData.id, { 
        name: editData.name, 
        type: editData.type,
        description: editData.description 
      });
      setIsEditModalOpen(false);
      setEditData(null);
      await onRefresh();
    } catch (err: any) {
      if (err instanceof ValidationError) {
        setEditError({
          message: err.message,
          details: err.details,
        });
      } else {
        setEditError({
          message: err.message || 'Failed to update profile type',
        });
      }
    } finally {
      setIsEditing(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteData) return;
    setDeleteError(null);
    try {
      setIsDeleting(true);
      await api.archiveEntityProfileType(deleteData.id);
      setIsDeleteOpen(false);
      setDeleteData(null);
      await onRefresh();
    } catch (err: any) {
      setDeleteError(err.message || 'Failed to delete profile type');
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
              onClick={() => navigate(`/u/profiles/${encodeURIComponent(pt.slug)}`)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  navigate(`/u/profiles/${encodeURIComponent(pt.slug)}`);
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
                    onClick={(e) => { 
                      e.stopPropagation(); 
                      setEditData(pt); 
                      setPersistedTypes([...pt.type]);
                      setIsEditModalOpen(true); 
                    }}
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
                  <label className="block text-sm font-medium text-gray-700 mb-1">Type Key(s)</label>
                  <TagInput
                    tags={createData.type}
                    onChange={tags => setCreateData({...createData, type: tags})}
                    placeholder="e.g. courier_id"
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    Press Enter, comma, or space to add a key. Keys must match how entities are referenced in thread steps.
                  </p>
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
              <div className="mt-8 flex justify-end items-center gap-6">
                <button
                  type="button"
                  onClick={() => setIsCreateModalOpen(false)}
                  className="text-red-700 hover:text-red-800 font-medium transition-colors text-sm"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isCreating}
                  className="px-8 py-3 bg-black rounded-xl text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50"
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
                onClick={() => { 
                  setIsEditModalOpen(false); 
                  setEditData(null);
                  setEditError(null);
                }}
                className="text-gray-400 hover:text-gray-600"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            <form onSubmit={handleEdit} className="p-6">
              {editError && (
                <Alert
                  type="error"
                  message={editError.message}
                  details={editError.details}
                  className="mb-4"
                  action={
                    isCreditError(editError.message)
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
                    value={editData.name}
                    onChange={e => setEditData({...editData, name: e.target.value})}
                    className="w-full px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Type Key(s)</label>
                  <TagInput
                    tags={editData.type}
                    persistedTags={persistedTypes}
                    onChange={tags => setEditData({...editData, type: tags})}
                    placeholder="Add more keys..."
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    Already persisted keys cannot be removed. You can add new keys.
                  </p>
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
              <div className="mt-8 flex justify-end items-center gap-6">
                <button
                  type="button"
                  onClick={() => { 
                    setIsEditModalOpen(false); 
                    setEditData(null);
                    setEditError(null);
                  }}
                  className="text-red-700 hover:text-red-800 font-medium transition-colors text-sm"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={isEditing}
                  className="px-8 py-3 bg-black rounded-xl text-white hover:bg-gray-800 transition-colors font-medium disabled:opacity-50"
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
              {deleteError && (
                <Alert
                  type="error"
                  message={deleteError}
                  className="mb-4"
                />
              )}
              <p className="text-gray-700 mb-6">
                Are you sure you want to delete the <strong>{deleteData.name}</strong> profile type? 
                This action cannot be undone and will prevent future threads from updating this profile.
              </p>
              <div className="flex justify-end items-center gap-6">
                <button
                  type="button"
                  onClick={() => { 
                    setIsDeleteOpen(false); 
                    setDeleteData(null);
                    setDeleteError(null);
                  }}
                  className="text-red-700 hover:text-red-800 font-medium transition-colors text-sm"
                >
                  Cancel
                </button>
                <button
                  onClick={confirmDelete}
                  disabled={isDeleting}
                  className="px-8 py-3 bg-red-600 text-white hover:bg-red-700 rounded-xl transition-colors font-medium disabled:opacity-50"
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

interface TagInputProps {
  tags: string[];
  persistedTags?: string[];
  onChange: (tags: string[]) => void;
  placeholder?: string;
}

function TagInput({ tags, persistedTags = [], onChange, placeholder }: TagInputProps) {
  const [inputValue, setInputValue] = useState('');

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',' || e.key === ' ') {
      e.preventDefault();
      const val = inputValue.trim().replace(/^,/, '');
      if (val && !tags.includes(val)) {
        onChange([...tags, val]);
      }
      setInputValue('');
    } else if (e.key === 'Backspace' && !inputValue && tags.length > 0) {
      const lastTag = tags[tags.length - 1];
      if (!persistedTags.includes(lastTag)) {
        onChange(tags.slice(0, -1));
      }
    }
  };

  const removeTag = (tagToRemove: string) => {
    if (persistedTags.includes(tagToRemove)) return;
    onChange(tags.filter(t => t !== tagToRemove));
  };

  return (
    <div className="w-full px-3 py-2 border border-gray-300 rounded focus-within:ring-1 focus-within:ring-black focus-within:border-black bg-white flex flex-wrap gap-2 items-center min-h-[42px]">
      {tags.map(tag => {
        const isPersisted = persistedTags.includes(tag);
        return (
          <span 
            key={tag} 
            className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-sm font-mono ${
              isPersisted ? 'bg-gray-100 text-gray-500 border border-gray-200' : 'bg-gray-900 text-white'
            }`}
          >
            {tag}
            {!isPersisted && (
              <button
                type="button"
                onClick={() => removeTag(tag)}
                className="hover:text-red-400 focus:outline-none ml-1"
              >
                <X className="w-3 h-3" />
              </button>
            )}
            {isPersisted && (
               <span className="w-3 h-3 flex items-center justify-center opacity-40">
                 <Database className="w-2.5 h-2.5" />
               </span>
            )}
          </span>
        );
      })}
      <input
        type="text"
        value={inputValue}
        onChange={e => setInputValue(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={tags.length === 0 ? placeholder : ''}
        className="flex-1 min-w-[120px] outline-none text-sm font-mono h-6 bg-transparent"
      />
    </div>
  );
}

import { useState, useEffect } from 'react';
import { useNavigate } from '@remix-run/react';
import type { MetaFunction } from "@remix-run/node";
import { api, type EntityProfileType } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import { Search, Plus, Database, ChevronRight, X, UserCircle, Edit2, Trash2 } from 'lucide-react';

export const meta: MetaFunction = () => {
  return [
    { title: "Entity Profiles - Threadify" },
    { name: "description", content: "Manage Entity Profile Schemas and View Profiles" },
  ];
};

export default function EntityProfiles() {
  const navigate = useNavigate();
  const [profileTypes, setProfileTypes] = useState<EntityProfileType[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Modal state
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const [createData, setCreateData] = useState({ name: '', type: '', description: '' });

  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
  const [editData, setEditData] = useState<EntityProfileType | null>(null);

  const [isDeleteOpen, setIsDeleteOpen] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteData, setDeleteData] = useState<EntityProfileType | null>(null);

  // Search state
  const [searchRef, setSearchRef] = useState('');
  const [searchType, setSearchType] = useState('');

  useEffect(() => {
    const token = api.getStoredToken();
    if (!token) {
      navigate('/login');
      return;
    }
    fetchProfileTypes();
  }, [navigate]);

  const fetchProfileTypes = async () => {
    try {
      setIsLoading(true);
      const res = await api.listEntityProfileTypes();
      setProfileTypes(res.data || []);
      setError(null);
    } catch (err: any) {
      console.error(err);
      setError(err.message || 'Failed to load profile types');
    } finally {
      setIsLoading(false);
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      setIsCreating(true);
      await api.createEntityProfileType(createData);
      setCreateData({ name: '', type: '', description: '' });
      setIsCreateModalOpen(false);
      await fetchProfileTypes();
    } catch (err: any) {
      alert(err.message || 'Failed to create profile type');
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
      await fetchProfileTypes();
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
      await fetchProfileTypes();
    } catch (err: any) {
      alert(err.message || 'Failed to delete profile type');
    } finally {
      setIsDeleting(false);
    }
  };

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (!searchRef || !searchType) return;
    navigate(`/u/profiles/${encodeURIComponent(searchType)}/${encodeURIComponent(searchRef)}`);
  };

  return (
    <AppLayout>
      <div className="p-8 px-4 sm:px-6 lg:px-8">
        <div className="mb-8 flex items-start justify-between">
          <div>
            <h1 className="text-3xl font-bold mb-2 flex items-center gap-3">
              <UserCircle className="w-8 h-8 text-black" />
              Entity Profiles
            </h1>
            <p className="text-gray-600">
              Define the schemas for your tracked entities and view their real-time health metrics.
            </p>
          </div>
          <button
            onClick={() => setIsCreateModalOpen(true)}
            className="px-4 py-2 bg-black text-white text-sm font-medium hover:bg-gray-800 transition-colors rounded flex items-center gap-2"
          >
            <Plus className="w-4 h-4" /> Create Profile Type
          </button>
        </div>

        {/* Search Section */}
        <div className="bg-gradient-to-r from-gray-50 to-white border border-gray-200 rounded-xl p-6 mb-8 shadow-sm">
          <h2 className="text-lg font-bold text-gray-900 mb-4 flex items-center gap-2">
            <Search className="w-5 h-5 text-gray-500" />
            Lookup Entity Profile
          </h2>
          <form onSubmit={handleSearch} className="flex gap-4 items-end">
            <div className="flex-1 max-w-sm">
              <label className="block text-sm font-medium text-gray-700 mb-1">Profile Type</label>
              <select
                value={searchType}
                onChange={(e) => setSearchType(e.target.value)}
                className="w-full h-11 px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none bg-white"
                required
              >
                <option value="">Select a type...</option>
                {profileTypes.map((pt) => (
                  <option key={pt.id} value={pt.type}>{pt.name} ({pt.type})</option>
                ))}
              </select>
            </div>
            <div className="flex-1 max-w-sm">
              <label className="block text-sm font-medium text-gray-700 mb-1">Reference Key (refKey)</label>
              <input
                type="text"
                placeholder="e.g. user-1234"
                value={searchRef}
                onChange={(e) => setSearchRef(e.target.value)}
                className="w-full h-11 px-3 py-2 border border-gray-300 rounded focus:ring-1 focus:ring-black focus:border-black outline-none bg-white"
                required
              />
            </div>
            <button
              type="submit"
              className="h-11 px-6 bg-black text-white font-medium hover:bg-gray-800 transition-colors rounded flex items-center justify-center"
              disabled={!searchRef || !searchType}
            >
              Search
            </button>
          </form>
        </div>

        {/* List Section */}
        <div className="bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-200 flex justify-between items-center bg-gray-50">
            <h2 className="text-lg font-bold text-gray-900 flex items-center gap-2">
              <Database className="w-5 h-5 text-gray-500" />
              Configured Profile Types
            </h2>
          </div>
          
          <div className="p-0">
            {isLoading ? (
              <div className="p-8 text-center text-gray-500">Loading profile types...</div>
            ) : error ? (
              <div className="p-8 text-center text-red-500">{error}</div>
            ) : profileTypes.length === 0 ? (
              <div className="p-12 text-center flex flex-col items-center">
                <Database className="w-12 h-12 text-gray-300 mb-3" />
                <h3 className="text-lg font-medium text-gray-900">No Profile Types Found</h3>
                <p className="text-gray-500 mt-1">Create your first Entity Profile Type to start tracking dimensions across workflows.</p>
              </div>
            ) : (
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="border-b border-gray-200 text-sm text-gray-500 bg-gray-50">
                    <th className="px-6 py-3 font-medium">Name</th>
                    <th className="px-6 py-3 font-medium">Type Key</th>
                    <th className="px-6 py-3 font-medium hidden md:table-cell">Description</th>
                    <th className="px-6 py-3 font-medium hidden sm:table-cell">Updated At</th>
                    <th className="px-6 py-3 font-medium text-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {profileTypes.map((pt) => (
                    <tr key={pt.id} className="border-b border-gray-100 hover:bg-gray-50 transition-colors">
                      <td className="px-6 py-4 font-medium text-gray-900">{pt.name}</td>
                      <td className="px-6 py-4 font-mono text-sm text-gray-500">{pt.type}</td>
                      <td className="px-6 py-4 text-sm text-gray-600 hidden md:table-cell">{pt.description || '-'}</td>
                      <td className="px-6 py-4 text-sm text-gray-500 hidden sm:table-cell">
                        {pt.updated_at ? new Date(pt.updated_at).toLocaleDateString() : '-'}
                      </td>
                      <td className="px-6 py-4 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <button
                            onClick={() => { setEditData(pt); setIsEditModalOpen(true); }}
                            className="p-1.5 text-gray-400 hover:text-black transition-colors rounded hover:bg-gray-100"
                            title="Edit"
                          >
                            <Edit2 className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => { setDeleteData(pt); setIsDeleteOpen(true); }}
                            className="p-1.5 text-gray-400 hover:text-red-600 transition-colors rounded hover:bg-red-50"
                            title="Delete"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      </div>

      {/* Create Modal */}
      {isCreateModalOpen && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4 backdrop-blur-sm">
          <div className="bg-white rounded-xl shadow-xl w-full max-w-md overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-200 flex justify-between items-center">
              <h2 className="text-lg font-bold text-gray-900">Create Profile Type</h2>
              <button 
                onClick={() => setIsCreateModalOpen(false)}
                className="text-gray-400 hover:text-gray-600"
              >
                <X className="w-5 h-5" />
              </button>
            </div>
            <form onSubmit={handleCreate} className="p-6">
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
    </AppLayout>
  );
}

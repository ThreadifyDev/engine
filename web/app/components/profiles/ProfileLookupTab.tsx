import { useState } from 'react';
import { useNavigate } from '@remix-run/react';
import type { EntityProfileType } from '~/lib/api';
import { Search } from 'lucide-react';

interface ProfileLookupTabProps {
  profileTypes: EntityProfileType[];
}

export default function ProfileLookupTab({ profileTypes }: ProfileLookupTabProps) {
  const navigate = useNavigate();
  const [searchRef, setSearchRef] = useState('');
  const [searchType, setSearchType] = useState('');

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    if (!searchRef || !searchType) return;
    navigate(`/u/profiles/${encodeURIComponent(searchType)}/${encodeURIComponent(searchRef)}`);
  };

  return (
    <>
      <div className="mb-6">
        <h2 className="text-lg font-semibold text-gray-900">Entity Profile Lookup</h2>
        <p className="text-sm text-gray-600 mt-1">
          Search for specific entity profiles by type and reference key
        </p>
      </div>

      <div className="bg-white border border-gray-200 rounded-lg p-6">
        <div className="flex items-center gap-2 mb-4">
          <Search className="w-5 h-5 text-gray-500" />
          <h3 className="font-medium text-gray-900">Search Profile</h3>
        </div>
        
        <form onSubmit={handleSearch} className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Profile Type</label>
              <select
                value={searchType}
                onChange={(e) => setSearchType(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
                required
              >
                <option value="">Select a type...</option>
                {profileTypes.map((pt) => 
                  pt.type.map((t) => (
                    <option key={`${pt.id}-${t}`} value={t}>
                      {pt.name} ({t})
                    </option>
                  ))
                )}
              </select>
            </div>
            
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Reference Key</label>
              <input
                type="text"
                placeholder="e.g. user-1234"
                value={searchRef}
                onChange={(e) => setSearchRef(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
                required
              />
            </div>
          </div>
          
          <div className="flex justify-end">
            <button
              type="submit"
              className="px-6 py-2 bg-gray-900 text-white font-medium hover:bg-gray-800 transition-colors rounded-md flex items-center gap-2"
              disabled={!searchRef || !searchType}
            >
              <Search className="w-4 h-4" />
              Search Profile
            </button>
          </div>
        </form>

        {profileTypes.length === 0 && (
          <div className="mt-6 p-4 bg-amber-50 border border-amber-200 rounded-md">
            <p className="text-sm text-amber-800">
              No profile types available. Create a profile type first to search for entity profiles.
            </p>
          </div>
        )}
      </div>
    </>
  );
}

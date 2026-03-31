import { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from '@remix-run/react';
import type { MetaFunction } from "@remix-run/node";
import { api } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import { APIKeysTab } from '~/components/developer/APIKeysTab';
import { ServiceAccountsTab } from '~/components/developer/ServiceAccountsTab';

export const meta: MetaFunction = () => {
  return [
    { title: "Developer - Threadify" },
    { name: "description", content: "Manage API keys and service accounts" },
  ];
};

export default function Developer() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [activeTab, setActiveTab] = useState<'api-keys' | 'service-accounts'>('api-keys');

  useEffect(() => {
    // Check authentication
    const token = api.getStoredToken();
    if (!token) {
      navigate('/login');
      return;
    }
  }, [navigate]);

  useEffect(() => {
    // Check for tab in query params
    const tabParam = searchParams.get('tab');
    if (tabParam === 'api-keys' || tabParam === 'service-accounts') {
      setActiveTab(tabParam as any);
    }
  }, [searchParams]);

  const handleTabChange = (tab: 'api-keys' | 'service-accounts') => {
    setActiveTab(tab);
    navigate(`/u/developer?tab=${tab}`, { replace: true });
  };

  return (
    <AppLayout>
      <div className="p-8 px-4 sm:px-6 lg:px-8">
        <div className="mb-8 flex items-start justify-between">
          <div>
            <h1 className="text-3xl font-bold mb-2">Developer Settings</h1>
            <p className="text-gray-600">
              Manage API keys and service accounts for programmatic access
            </p>
          </div>
          <div>
            {activeTab === 'api-keys' && (
              <button
                onClick={() => {
                  const event = new CustomEvent('create-api-key');
                  window.dispatchEvent(event);
                }}
                className="px-4 py-2 bg-black text-white text-sm font-medium hover:bg-gray-800 transition-colors rounded"
              >
                + Create New API Key
              </button>
            )}
            {activeTab === 'service-accounts' && (
              <button
                onClick={() => {
                  const event = new CustomEvent('create-service-account');
                  window.dispatchEvent(event);
                }}
                className="px-4 py-2 bg-black text-white text-sm font-medium hover:bg-gray-800 transition-colors rounded"
              >
                + New Service Account
              </button>
            )}
          </div>
        </div>

        {/* Tab Navigation */}
        <div className="mb-8">
          <div className="flex gap-4">
            <button
              onClick={() => handleTabChange('api-keys')}
              className={`px-6 py-3 font-medium transition-colors rounded-lg ${
                activeTab === 'api-keys'
                  ? 'bg-black text-white -mb-0.5'
                  : 'text-gray-800 hover:text-black'
              }`}
            >
              API Keys
            </button>
            <button
              onClick={() => handleTabChange('service-accounts')}
              className={`px-6 py-3 font-medium transition-colors rounded-lg ${
                activeTab === 'service-accounts'
                  ? 'bg-black text-white -mb-0.5'
                  : 'text-gray-800 hover:text-black'
              }`}
            >
              Service Accounts
            </button>
          </div>
        </div>

        {/* Tab Content */}
        <div>
          {activeTab === 'api-keys' && <APIKeysTab />}
          {activeTab === 'service-accounts' && <ServiceAccountsTab />}
        </div>
      </div>
    </AppLayout>
  );
}

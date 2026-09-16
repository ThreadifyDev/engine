import { TabBar } from '~/components/TabBar';
import { useState, useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { api } from '~/lib/api';
import AppLayout from '~/components/AppLayout';
import { APIKeysTab } from '~/components/developer/APIKeysTab';
import { ServiceAccountsTab } from '~/components/developer/ServiceAccountsTab';


export default function Developer() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [activeTab, setActiveTab] = useState<'api-keys' | 'service-accounts'>('api-keys');

  useEffect(() => {
    // Check authentication
    const token = api.isAuthenticated();
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
      <div className="min-w-0 p-4 sm:p-6 lg:p-8">
        <div className="mb-8 flex items-start justify-between">
          <div>
            <h1 className="text-2xl font-bold mb-2">Developer Settings</h1>
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

        <TabBar label="Developer" value={activeTab} onChange={handleTabChange} panelId="developer-panel" className="mb-6"
          items={[{value:'api-keys',label:'API Keys'},{value:'service-accounts',label:'Service Accounts'}]} />

        {/* Tab Content */}
        <div id="developer-panel" role="tabpanel" aria-label="Developer content">
          {activeTab === 'api-keys' && <APIKeysTab />}
          {activeTab === 'service-accounts' && <ServiceAccountsTab />}
        </div>
      </div>
    </AppLayout>
  );
}

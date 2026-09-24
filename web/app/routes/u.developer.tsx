import { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { Plus } from 'lucide-react';
import { TabBar } from '~/components/TabBar';
import { api } from '~/lib/api';
import WorkspacePage from '~/components/WorkspacePage';
import { APIKeysTab } from '~/components/developer/APIKeysTab';
import { ServiceAccountsTab } from '~/components/developer/ServiceAccountsTab';

export default function Developer() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [activeTab, setActiveTab] = useState<'api-keys' | 'service-accounts'>('api-keys');

  useEffect(() => {
    if (!api.isAuthenticated()) navigate('/login');
  }, [navigate]);

  useEffect(() => {
    const tab = searchParams.get('tab');
    if (tab === 'api-keys' || tab === 'service-accounts') setActiveTab(tab);
  }, [searchParams]);

  const handleTabChange = (tab: 'api-keys' | 'service-accounts') => {
    setActiveTab(tab);
    navigate('/u/developer?tab=' + tab, { replace: true });
  };

  const create = () => {
    window.dispatchEvent(new CustomEvent(activeTab === 'api-keys' ? 'create-api-key' : 'create-service-account'));
  };

  return (
    <WorkspacePage eyebrow="Workspace access" title="Developer"
      description="Manage the credentials and service identities your integrations use."
      actions={<button type="button" onClick={create} className="inline-flex items-center gap-2 rounded-lg bg-stone-950 px-4 py-2.5 text-sm font-medium text-white shadow-sm hover:bg-stone-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-emerald-700">
        <Plus className="h-4 w-4" /> {activeTab === 'api-keys' ? 'New API key' : 'New service account'}
      </button>}>
      <TabBar label="Developer" value={activeTab} onChange={handleTabChange} panelId="developer-panel" className="mb-6"
        items={[{ value: 'api-keys', label: 'API keys' }, { value: 'service-accounts', label: 'Service accounts' }]} />
      <div id="developer-panel" role="tabpanel" aria-label="Developer content">
        {activeTab === 'api-keys' ? <APIKeysTab /> : <ServiceAccountsTab />}
      </div>
    </WorkspacePage>
  );
}

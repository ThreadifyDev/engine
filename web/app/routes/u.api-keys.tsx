import { useEffect } from 'react';
import { useNavigate } from 'react-router';
import { Plus } from 'lucide-react';
import { APIKeysTab } from '~/components/developer/APIKeysTab';
import WorkspacePage from '~/components/WorkspacePage';
import { api } from '~/lib/api';

export default function APIKeys() {
  const navigate = useNavigate();
  const authenticated = api.isAuthenticated();

  useEffect(() => {
    if (!authenticated) navigate('/login');
  }, [authenticated, navigate]);

  if (!authenticated) return null;

  const openCreateModal = () => window.dispatchEvent(new CustomEvent('create-api-key'));

  return (
    <WorkspacePage eyebrow="Workspace / Developer" title="API keys"
      description="Manage credentials for the Threadify API."
      actions={<button type="button" onClick={openCreateModal}
        className="inline-flex items-center gap-2 rounded-lg bg-stone-950 px-4 py-2.5 text-sm font-medium text-white shadow-sm hover:bg-stone-800">
        <Plus className="h-4 w-4" /> New API key
      </button>}>
      <APIKeysTab />
    </WorkspacePage>
  );
}

import { useState, useEffect, useRef } from 'react';
import { useNavigate, useParams } from '@remix-run/react';
import { api } from '~/lib/api';
import SideNav from '~/components/SideNav';

type TabType = 'mermaid' | 'yaml';

export default function ContractVersionDetail() {
  const navigate = useNavigate();
  const { id, version } = useParams();
  const [versionData, setVersionData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [activeTab, setActiveTab] = useState<TabType>('mermaid');
  const [mermaidLoaded, setMermaidLoaded] = useState(false);
  const mermaidRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    // Dynamically import mermaid only on client side
    if (typeof window !== 'undefined') {
      import('mermaid').then((m) => {
        m.default.initialize({
          startOnLoad: false,
          theme: 'default',
          securityLevel: 'loose',
        });
        setMermaidLoaded(true);
      });
    }
  }, []);

  useEffect(() => {
    // Check authentication
    const token = api.getStoredToken();
    if (!token) {
      navigate('/auth/login');
      return;
    }

    if (id && version) {
      fetchVersion();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, version]);

  useEffect(() => {
    // Render mermaid when tab changes to mermaid and data is loaded
    if (activeTab === 'mermaid' && versionData?.mermaid && mermaidLoaded && mermaidRef.current) {
      import('mermaid').then(async (m) => {
        const element = mermaidRef.current;
        if (element) {
          try {
            // Clear previous content
            element.innerHTML = '';
            
            // Create a unique ID for this diagram
            const id = `mermaid-${Date.now()}`;
            
            // Render mermaid diagram
            const { svg } = await m.default.render(id, versionData.mermaid);
            element.innerHTML = svg;
          } catch (error) {
            console.error('Mermaid rendering error:', error);
            element.innerHTML = '<div class="text-red-600 p-4">Failed to render diagram. Please check the contract definition.</div>';
          }
        }
      });
    }
  }, [activeTab, versionData, mermaidLoaded]);

  const fetchVersion = async () => {
    try {
      setLoading(true);
      setError('');
      const response = await api.getContractVersion(id!, version!);
      setVersionData(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load contract version');
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex h-screen bg-white">
        <SideNav />
        <div className="flex-1 flex items-center justify-center ml-64">
          <div className="text-center">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-black"></div>
            <p className="mt-4 text-black">Loading version...</p>
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-screen bg-white">
        <SideNav />
        <div className="flex-1 flex items-center justify-center ml-64">
          <div className="text-center">
            <p className="text-red-600 mb-4">{error}</p>
            <button
              onClick={() => navigate(`/contracts/${id}`)}
              className="px-4 py-2 bg-black text-white hover:bg-gray-800"
            >
              Back to Contract
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-screen bg-white">
      <SideNav />
      <div className="flex-1 overflow-auto ml-64">
        <div className="p-8">
          {/* Header */}
          <div className="mb-8">
            <button
              onClick={() => navigate(`/contracts/${id}`)}
              className="text-black hover:underline mb-4 flex items-center"
            >
              ← Back to Contract
            </button>
            <h1 className="text-4xl font-bold text-black mb-4" style={{ fontFamily: 'Block, sans-serif' }}>
              Contract {version}
            </h1>
            <p className="text-gray-600 mt-2">
              Created: {versionData?.createdAt ? new Date(versionData.createdAt).toLocaleDateString() : 'N/A'}
            </p>
          </div>

          {/* Metadata Section */}
          {versionData && (
            <div className="mb-8 border-2 border-black p-6 bg-gray-50">
              <h2 className="text-xl font-bold text-black mb-4" style={{ fontFamily: 'Block, sans-serif' }}>
                Metadata
              </h2>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-sm text-gray-600">Version</p>
                  <p className="text-black font-medium">{versionData.version}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-600">Contract ID</p>
                  <p className="text-black font-medium">{versionData.contractId}</p>
                </div>
                <div>
                  <p className="text-sm text-gray-600">Created At</p>
                  <p className="text-black font-medium">
                    {versionData.createdAt ? new Date(versionData.createdAt).toLocaleString() : 'N/A'}
                  </p>
                </div>
                {versionData.createdBy && (
                  <div>
                    <p className="text-sm text-gray-600">Created By</p>
                    <p className="text-black font-medium">{versionData.createdBy}</p>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Tabbed View */}
          <div className="bg-gray-50 border-2 border-black">
            {/* Tab Headers */}
            <div className="flex border-b-2 border-black">
              <button
                onClick={() => setActiveTab('mermaid')}
                className={`flex-1 px-6 py-4 font-bold transition-colors ${
                  activeTab === 'mermaid'
                    ? 'bg-black text-white'
                    : 'bg-white text-black hover:bg-gray-100'
                }`}
                style={{ fontFamily: 'Block, sans-serif' }}
              >
                Mermaid Chart
              </button>
              <button
                onClick={() => setActiveTab('yaml')}
                className={`flex-1 px-6 py-4 font-bold transition-colors border-l-2 border-black ${
                  activeTab === 'yaml'
                    ? 'bg-black text-white'
                    : 'bg-white text-black hover:bg-gray-100'
                }`}
                style={{ fontFamily: 'Block, sans-serif' }}
              >
                YAML
              </button>
            </div>

            {/* Tab Content */}
            <div className="p-6">
              {activeTab === 'mermaid' && (
                <div>
                  {versionData?.mermaid ? (
                    <div className="bg-white border-2 border-black p-6 overflow-x-auto">
                      <div ref={mermaidRef} className="mermaid">
                        {/* Mermaid diagram will be rendered here */}
                      </div>
                    </div>
                  ) : (
                    <div className="bg-white border-2 border-black p-6 text-center text-gray-600">
                      No Mermaid diagram available for this version
                    </div>
                  )}
                </div>
              )}

              {activeTab === 'yaml' && (
                <div>
                  <div className="flex items-center justify-between mb-4">
                    <h3 className="text-lg font-bold text-black" style={{ fontFamily: 'Block, sans-serif' }}>
                      Contract Definition
                    </h3>
                    <button
                      onClick={() => {
                        navigator.clipboard.writeText(versionData?.yamlContent || '');
                        alert('YAML copied to clipboard!');
                      }}
                      className="px-4 py-2 border-2 border-black hover:bg-black hover:text-white transition-colors text-sm"
                    >
                      Copy YAML
                    </button>
                  </div>
                  <pre className="bg-white border-2 border-black p-4 overflow-x-auto">
                    <code className="text-sm text-black font-mono">
                      {versionData?.yamlContent || 'No YAML content available'}
                    </code>
                  </pre>
                </div>
              )}
            </div>
          </div>

        </div>
      </div>
    </div>
  );
}

import { useState, useEffect } from 'react';
import { useNavigate, useParams } from '@remix-run/react';
import { useQuery } from '@tanstack/react-query';
import { api } from '~/lib/api';
import { graphqlClient } from '~/lib/graphql';
import SideNav from '~/components/SideNav';
import ContractGraphView from '~/components/ContractGraphView';

type TabType = 'diagram' | 'yaml';

export default function ContractVersionDetail() {
  const navigate = useNavigate();
  const { id, version } = useParams();
  const [versionData, setVersionData] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [activeTab, setActiveTab] = useState<TabType>('diagram');

  // Contract graph is already in versionData.graph
  const contractGraph = versionData?.graph;
  const graphLoading = loading;
  const graphError = error;

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
        <div className="flex-1 flex items-center justify-center ml-16">
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
        <div className="flex-1 flex items-center justify-center ml-16">
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
      <div className="flex-1 overflow-auto ml-16">
        <div className="p-8">
          {/* Header */}
          <div className="mb-8">
            <button
              onClick={() => navigate(`/contracts/${id}`)}
              className="text-gray-600 hover:text-gray-900 mb-6 flex items-center text-sm"
            >
              ← Back to Contract
            </button>
            
            {loading ? (
              <div className="flex items-center gap-2">
                <div className="inline-block animate-spin rounded-full h-5 w-5 border-b-2 border-gray-900"></div>
                <h1 className="text-2xl font-bold text-gray-900">Loading...</h1>
              </div>
            ) : (
              <>
                {/* Single line header like thread view */}
                <h1 className="text-2xl font-bold text-gray-900 mb-4">
                  {versionData?.contractName || 'Unknown Contract'}
                </h1>
                
                {/* Metadata inline */}
                <div className="flex items-center gap-4 text-sm text-gray-600 mb-3">
                  <span>
                    Created <span className="text-gray-900 font-medium">
                      {versionData?.createdAt ? new Date(versionData.createdAt).toLocaleDateString('en-US', {
                        month: 'short',
                        day: 'numeric',
                        year: 'numeric'
                      }) : 'N/A'}
                    </span>
                  </span>
                  <span className="text-gray-300">|</span>
                  <span>
                    Version <span className="text-gray-900 font-medium">v{version}</span>
                  </span>
                </div>

                {/* Validation metadata */}
                {versionData?.graph?.graph?.validation && (
                  <div className="flex items-center gap-3 text-xs">
                    {versionData.graph.graph.validation.MaxDuration && (
                      <span className="px-2.5 py-1 bg-blue-50 text-blue-700 rounded border border-blue-200">
                        Max Duration: <span className="font-semibold">{versionData.graph.graph.validation.MaxDuration}</span>
                      </span>
                    )}
                    {versionData.graph.graph.validation.MultipleTerminalsSeverity && (
                      <span className="px-2.5 py-1 bg-amber-50 text-amber-700 rounded border border-amber-200">
                        Multiple Terminals: <span className="font-semibold">{versionData.graph.graph.validation.MultipleTerminalsSeverity}</span>
                      </span>
                    )}
                  </div>
                )}
              </>
            )}
          </div>


          {/* Tabbed View */}
          <div>
            {/* Tab Headers */}
            <div className="flex border-b border-gray-200 mb-6">
              <button
                onClick={() => setActiveTab('diagram')}
                className={`px-4 py-2 text-sm font-medium transition-colors ${
                  activeTab === 'diagram'
                    ? 'text-gray-900 border-b-2 border-gray-900'
                    : 'text-gray-500 hover:text-gray-700'
                }`}
              >
                Graph
              </button>
              <button
                onClick={() => setActiveTab('yaml')}
                className={`px-4 py-2 text-sm font-medium transition-colors ml-6 ${
                  activeTab === 'yaml'
                    ? 'text-gray-900 border-b-2 border-gray-900'
                    : 'text-gray-500 hover:text-gray-700'
                }`}
              >
                YAML
              </button>
            </div>

            {/* Tab Content */}
            <div>
              {activeTab === 'diagram' && (
                <div>
                  {graphLoading ? (
                    <div className="bg-white border border-gray-200 rounded p-12 text-center">
                      <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-gray-900"></div>
                      <p className="mt-4 text-gray-600">Loading contract graph...</p>
                    </div>
                  ) : graphError ? (
                    <div className="bg-red-50 border border-red-200 rounded p-6 text-center">
                      <p className="text-red-600 font-medium mb-2">Failed to load contract graph</p>
                      <p className="text-sm text-red-500">{graphError}</p>
                    </div>
                  ) : contractGraph ? (
                    <ContractGraphView
                      contractName={versionData?.contractName || 'Contract'}
                      version={parseInt(version!)}
                      graphData={contractGraph}
                    />
                  ) : (
                    <div className="bg-white border border-gray-200 rounded p-6 text-center text-gray-500">
                      <p>No diagram available for this version</p>
                    </div>
                  )}
                </div>
              )}

              {activeTab === 'yaml' && (
                <div>
                  <div className="flex items-center justify-between mb-4">
                    <h3 className="text-sm font-medium text-gray-900">
                      Contract Definition
                    </h3>
                    <button
                      onClick={() => {
                        navigator.clipboard.writeText(versionData?.yamlContent || '');
                        alert('YAML copied to clipboard!');
                      }}
                      className="px-3 py-1.5 border border-gray-300 rounded hover:bg-gray-50 transition-colors text-sm text-gray-700"
                    >
                      Copy YAML
                    </button>
                  </div>
                  <pre className="bg-white border border-gray-200 rounded p-4 overflow-x-auto overflow-y-auto h-[calc(100vh-300px)]">
                    <code className="text-sm text-gray-900 font-mono">
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

import { useState, useEffect, useRef } from 'react';
import { graphqlClient, type Thread } from '~/lib/graphql';
import { Activity, Search, Calendar } from 'lucide-react';
import { formatDistanceToNow } from 'date-fns';

export default function HistoryTab({
  profileId,
  refValue,
  navigate,
}: {
  profileId: string;
  refValue: string;
  navigate: (path: string) => void;
}) {
  const [threads, setThreads] = useState<Thread[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const loadedRef = useRef(false);

  useEffect(() => {
    if (loadedRef.current) return;
    loadedRef.current = true;

    (async () => {
      try {
        setIsLoading(true);
        setError(null);
        // Query by profileId (backend resolves the reference keys)
        const res = await graphqlClient.getEntityProfileHistory({
          profileID: profileId,
        });
        setThreads(res.threads || []);
        setTotalCount(res.totalCount || 0);
      } catch (err: any) {
        setError(err.message || 'Failed to load thread history');
      } finally {
        setIsLoading(false);
      }
    })();
  }, [profileId, refValue]);

  if (isLoading) {
    return (
      <div className="bg-white border border-gray-200 rounded-lg p-12 text-center text-gray-500 flex items-center justify-center gap-2">
        <Activity className="w-5 h-5 animate-pulse" /> Loading thread history…
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-8 text-center text-red-600">
        {error}
      </div>
    );
  }

  if (threads.length === 0) {
    return (
      <div className="bg-white border border-gray-200 rounded-lg p-12 text-center flex flex-col items-center">
        <Search className="w-12 h-12 text-gray-300 mb-3" />
        <h3 className="text-lg font-medium text-gray-900">No threads referenced this entity</h3>
        <p className="text-gray-500 mt-1 max-w-md">
          No threads have been recorded with <code className="font-mono text-xs bg-gray-100 px-1.5 py-0.5 rounded">ID={refValue}</code>.
        </p>
      </div>
    );
  }

  const getStatusBadge = (status: string) => {
    const base = 'text-xs px-1.5 py-0.5 rounded-md font-medium';
    const map: Record<string, string> = {
      active: `${base} bg-blue-50 text-blue-700`,
      completed: `${base} bg-green-50 text-green-700`,
      failed: `${base} bg-red-50 text-red-700`,
      cancelled: `${base} bg-gray-100 text-gray-700`,
    };
    return map[status] || `${base} bg-gray-100 text-gray-700`;
  };

  return (
    <div className="space-y-2">
      <div className="mb-3 text-xs text-gray-600">
        Found <span className="font-semibold text-gray-900">{totalCount}</span> thread
        {totalCount !== 1 ? 's' : ''} referencing this entity
      </div>

      {threads.map((thread) => {
        const refs = thread.refs
          ? typeof thread.refs === 'string'
            ? JSON.parse(thread.refs)
            : thread.refs
          : {};

        const threadIdSummary = thread.id.split('-').pop() || '';
        const threadTitle = thread.label 
          ? `${thread.label} (${threadIdSummary})` 
          : thread.contractName 
            ? `${thread.contractName} (${threadIdSummary})` 
            : threadIdSummary;

        return (
          <div
            key={thread.id}
            onClick={() => navigate(`/u/threads/${thread.id}`)}
            className="bg-white border border-gray-200 rounded-lg p-3 hover:shadow-sm hover:border-gray-300 transition-all cursor-pointer"
          >
            <div className="flex justify-between items-start">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1.5">
                  <h3 className="font-semibold text-sm text-gray-900 truncate">
                    {threadTitle}
                  </h3>
                  <span className={getStatusBadge(thread.status)}>{thread.status}</span>
                </div>
                <p className="text-xs text-gray-500 font-mono mb-2 truncate">
                  {thread.contractName && thread.contractVersion
                    ? `${thread.contractName} v${thread.contractVersion}`
                    : '-'}
                </p>
                <div className="flex items-center gap-3 text-xs text-gray-600">
                  {thread.startedAt && (
                    <div className="flex items-center gap-1">
                      <Calendar className="w-3 h-3" />
                      {formatDistanceToNow(new Date(thread.startedAt), { addSuffix: true })}
                    </div>
                  )}
                  {Object.keys(refs).length > 0 && (
                    <div className="flex items-center gap-1.5">
                      {Object.entries(refs)
                        .slice(0, 2)
                        .map(([key, value]) => (
                          <span
                            key={key}
                            className="inline-flex items-center px-1.5 py-0.5 bg-blue-50 text-blue-700 text-xs rounded"
                          >
                            <span className="font-medium">{key}:</span>
                            <span className="ml-0.5">{String(value)}</span>
                          </span>
                        ))}
                      {Object.keys(refs).length > 2 && (
                        <span className="text-xs text-gray-500">
                          +{Object.keys(refs).length - 2} more
                        </span>
                      )}
                    </div>
                  )}
                </div>
              </div>
              <div className="ml-3 flex-shrink-0">
                <button className="px-3 py-1.5 text-xs font-medium text-gray-700 hover:text-gray-900 hover:bg-gray-50 rounded-md transition-colors">
                  View →
                </button>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

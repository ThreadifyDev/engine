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
      <div className="flex items-center justify-center gap-2 rounded-lg border border-gray-200 bg-white p-6 text-center text-gray-500 sm:p-12">
        <Activity className="w-5 h-5 animate-pulse" /> Loading thread history…
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-lg border border-red-200 bg-red-50 p-5 text-center text-red-600 sm:p-8">
        {error}
      </div>
    );
  }

  if (threads.length === 0) {
    return (
      <div className="flex flex-col items-center rounded-lg border border-gray-200 bg-white p-6 text-center sm:p-12">
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
    <div className="min-w-0 space-y-3">
      <div className="mb-3 text-xs text-gray-700">
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
            className="min-w-0 cursor-pointer rounded-lg border border-gray-200 bg-white p-3 transition-all hover:border-gray-300 hover:shadow-sm sm:p-4"
          >
            <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="flex-1 min-w-0">
                <div className="mb-1.5 flex min-w-0 flex-wrap items-center gap-2">
                  <h3 className="min-w-0 break-words text-sm font-semibold text-gray-900 sm:truncate">
                    {threadTitle}
                  </h3>
                  <span className={getStatusBadge(thread.status)}>{thread.status}</span>
                </div>
                <p className="mb-2 break-all font-mono text-xs text-gray-600 sm:truncate">
                  {thread.contractName && thread.contractVersion
                    ? `${thread.contractName} v${thread.contractVersion}`
                    : '-'}
                </p>
                <div className="flex min-w-0 flex-col items-start gap-2 text-xs text-gray-700 sm:flex-row sm:flex-wrap sm:items-center sm:gap-3">
                  {thread.startedAt && (
                    <div className="flex items-center gap-1 whitespace-normal">
                      <Calendar className="h-3 w-3 flex-shrink-0" />
                      {formatDistanceToNow(new Date(thread.startedAt), { addSuffix: true })}
                    </div>
                  )}
                  {Object.keys(refs).length > 0 && (
                    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                      {Object.entries(refs)
                        .slice(0, 2)
                        .map(([key, value]) => (
                          <span
                            key={key}
                            className="inline-flex max-w-full min-w-0 flex-wrap items-center rounded border border-gray-300 bg-gray-100 px-1.5 py-0.5 text-xs"
                          >
                            <span className="break-all font-semibold text-gray-700">{key}:</span>
                            <span className="ml-0.5 break-all font-mono font-medium text-gray-900">{String(value)}</span>
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
              <div className="flex flex-shrink-0 justify-end sm:ml-3">
                <button className="rounded-md bg-gray-50 px-3 py-1.5 text-xs font-semibold text-gray-800 transition-colors hover:bg-gray-100 hover:text-gray-950 sm:bg-transparent">
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

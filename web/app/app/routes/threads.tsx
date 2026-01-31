import { useNavigate } from '@remix-run/react';
import { useQuery } from '@tanstack/react-query';
import { graphqlClient, type Thread } from '~/lib/graphql';
import { formatDistanceToNow } from 'date-fns';
import { CheckCircle2, XCircle, Clock, AlertCircle, Search, Filter } from 'lucide-react';
import { useState } from 'react';
import AppLayout from '~/components/AppLayout';

export default function ThreadsPage() {
  const navigate = useNavigate();
  const [statusFilter, setStatusFilter] = useState<string>('');
  const [searchQuery, setSearchQuery] = useState('');

  const { data: threads = [], isLoading, error } = useQuery({
    queryKey: ['threads', statusFilter],
    queryFn: () => graphqlClient.getThreads({
      status: statusFilter || undefined,
      limit: 50,
    }),
  });

  const filteredThreads = threads.filter(thread => {
    if (!searchQuery) return true;
    const query = searchQuery.toLowerCase();
    return (
      thread.id.toLowerCase().includes(query) ||
      thread.contractName?.toLowerCase().includes(query) ||
      JSON.stringify(thread.refs || {}).toLowerCase().includes(query)
    );
  });

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'active':
        return <Clock className="w-5 h-5 text-black" />;
      case 'completed':
        return <CheckCircle2 className="w-5 h-5 text-green-600" />;
      case 'failed':
        return <XCircle className="w-5 h-5 text-red-600" />;
      case 'cancelled':
        return <AlertCircle className="w-5 h-5 text-gray-600" />;
      default:
        return <Clock className="w-5 h-5 text-gray-400" />;
    }
  };

  const getStatusBadge = (status: string) => {
    const baseClasses = 'text-xs px-2 py-0.5 rounded-full border';
    const configs = {
      active: `${baseClasses} bg-blue-50 text-blue-700 border-blue-200`,
      completed: `${baseClasses} bg-green-50 text-green-700 border-green-200`,
      failed: `${baseClasses} bg-red-50 text-red-700 border-red-200`,
      cancelled: `${baseClasses} bg-gray-50 text-gray-700 border-gray-200`,
    };
    return configs[status as keyof typeof configs] || `${baseClasses} bg-gray-50 text-gray-700 border-gray-200`;
  };

  return (
    <AppLayout>
      <div className="p-12">
        {/* Header */}
        <div className="mb-8">
          <h1 className="text-4xl font-bold text-black mb-2">Threads</h1>
          <p className="text-gray-600">Monitor and manage your workflow instances</p>
        </div>

        {/* Filters */}
        <div className="mb-6 flex gap-4">
          <div className="flex-1 relative">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 w-5 h-5" />
            <input
              type="text"
              placeholder="Search by thread ID, contract, or references..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full pl-10 pr-4 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
            />
          </div>

          <div className="relative">
            <Filter className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 w-5 h-5" />
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="pl-10 pr-8 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent appearance-none bg-white"
            >
              <option value="">All Statuses</option>
              <option value="active">Active</option>
              <option value="completed">Completed</option>
              <option value="failed">Failed</option>
              <option value="cancelled">Cancelled</option>
            </select>
          </div>
        </div>

        {/* Loading State */}
        {isLoading && (
          <div className="flex items-center justify-center py-12">
            <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-black"></div>
          </div>
        )}

        {/* Error State */}
        {error && (
          <div className="border border-red-200 rounded-lg bg-red-50 p-6">
            <h3 className="text-red-800 font-bold mb-2">Error loading threads</h3>
            <p className="text-red-600">{(error as Error).message}</p>
          </div>
        )}

        {/* Empty State */}
        {!isLoading && !error && filteredThreads.length === 0 && (
          <div className="border border-gray-200 rounded-lg p-12 text-center bg-white shadow-sm">
            <Clock className="w-16 h-16 text-gray-400 mx-auto mb-4" />
            <h3 className="text-xl font-bold text-gray-900 mb-2">No threads found</h3>
            <p className="text-gray-600">
              {searchQuery || statusFilter
                ? 'Try adjusting your filters'
                : 'Start a thread to see it here'}
            </p>
          </div>
        )}

        {/* Threads List */}
        {!isLoading && !error && filteredThreads.length > 0 && (
          <div className="space-y-4">
            {filteredThreads.map((thread) => (
              <div
                key={thread.id}
                onClick={() => navigate(`/threads/${thread.id}`)}
                className="bg-white border-b border-gray-200 p-6 hover:bg-gray-50 transition-all cursor-pointer"
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      {getStatusIcon(thread.status)}
                      <h3 className="font-bold text-black text-lg">{thread.id}</h3>
                      <span className={`px-3 py-1 text-sm font-medium ${getStatusBadge(thread.status)}`}>
                        {thread.status}
                      </span>
                    </div>

                    <div className="space-y-1 text-sm text-gray-600">
                      {thread.contractName && (
                        <div>
                          <span className="font-medium">Contract:</span> {thread.contractName}
                          {thread.contractVersion && (
                            <span className="text-gray-400 ml-1">v{thread.contractVersion}</span>
                          )}
                        </div>
                      )}

                      {thread.startedAt && (
                        <div>
                          <span className="font-medium">Started:</span>{' '}
                          {formatDistanceToNow(new Date(thread.startedAt), { addSuffix: true })}
                        </div>
                      )}

                      {thread.completedAt && (
                        <div>
                          <span className="font-medium">Completed:</span>{' '}
                          {formatDistanceToNow(new Date(thread.completedAt), { addSuffix: true })}
                        </div>
                      )}

                      {thread.error && (
                        <div className="text-red-600">
                          <span className="font-medium">Error:</span> {thread.error}
                        </div>
                      )}

                      {thread.refs && Object.keys(thread.refs).length > 0 && (
                        <div className="flex flex-wrap gap-2 mt-2">
                          {Object.entries(thread.refs).slice(0, 3).map(([key, value]) => (
                            <span
                              key={key}
                              className="inline-flex items-center px-2 py-1 bg-gray-100 text-xs font-mono"
                            >
                              {key}: {String(value)}
                            </span>
                          ))}
                          {Object.keys(thread.refs).length > 3 && (
                            <span className="inline-flex items-center px-2 py-1 bg-gray-100 text-xs">
                              +{Object.keys(thread.refs).length - 3} more
                            </span>
                          )}
                        </div>
                      )}
                    </div>
                  </div>

                  <div className="text-right">
                    <button className="px-4 py-2 bg-gray-50 text-gray-700 text-sm font-medium rounded-lg border border-gray-200 hover:bg-gray-100 transition-colors">
                      View Details →
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Pagination Info */}
        {!isLoading && !error && filteredThreads.length > 0 && (
          <div className="mt-6 text-center text-sm text-gray-600">
            Showing {filteredThreads.length} thread{filteredThreads.length !== 1 ? 's' : ''}
          </div>
        )}
      </div>
    </AppLayout>
  );
}

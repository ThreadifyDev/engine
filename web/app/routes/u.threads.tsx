import { useNavigate, useSearchParams } from '@remix-run/react';
import { useState, useEffect } from 'react';
import { Search, Filter, ChevronDown, ChevronUp, X, Calendar, Hash, FileText, ChevronLeft, ChevronRight, MessageSquare, Send, User, Bot, Sparkles, PlusCircle, MessageCircle } from 'lucide-react';
import AppLayout from '~/components/AppLayout';
import { graphqlClient, type Thread } from '~/lib/graphql';
import { api } from '~/lib/api';
import { formatDistanceToNow } from 'date-fns';
import ThreadChat from '~/components/ThreadChat';

type SearchMode = 'advanced' | 'chat';

interface RefFilter {
  key: string;
  value: string;
}

interface SearchFilters {
  threadId?: string;
  contractName?: string;
  contractVersion?: number;
  refs: RefFilter[];
  timeRange: 'all' | '24h' | '7d' | '30d' | 'custom';
  startedAfter?: string;
  startedBefore?: string;
  status?: string;
}

export default function ThreadsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  
  const [searchMode, setSearchMode] = useState<SearchMode>('advanced');
  const [filters, setFilters] = useState<SearchFilters>({
    refs: [],
    timeRange: 'all',
  });
  
  const [searchResults, setSearchResults] = useState<Thread[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [currentPage, setCurrentPage] = useState(1);
  const [totalResults, setTotalResults] = useState(0);
  const resultsPerPage = 20;

  // Restore search state from URL params on mount
  useEffect(() => {
    const mode = searchParams.get('mode') as SearchMode;
    const page = searchParams.get('page');
    const contractName = searchParams.get('contract');
    const contractVersion = searchParams.get('version');
    const threadId = searchParams.get('threadId');
    const status = searchParams.get('status');
    const timeRange = searchParams.get('timeRange');
    const refKey = searchParams.get('refKey');
    const refValue = searchParams.get('refValue');

    if (mode) setSearchMode(mode);
    if (page) setCurrentPage(parseInt(page));

    if (mode === 'advanced') {
      const restoredFilters: SearchFilters = {
        refs: refKey && refValue ? [{ key: refKey, value: refValue }] : [],
        timeRange: (timeRange as any) || 'all',
        threadId,
        contractName,
        contractVersion: contractVersion ? parseInt(contractVersion) : undefined,
        status,
      };
      setFilters(restoredFilters);
    }
  }, []);

  // Re-execute search when URL params change (after initial mount)
  useEffect(() => {
    const mode = searchParams.get('mode') as SearchMode;
    const page = parseInt(searchParams.get('page') || '1');

    if (!mode) return;

    if (mode === 'advanced') {
      performAdvancedSearch(page);
    }
  }, [searchParams]);

  const performAdvancedSearch = async (page: number = 1, searchFilters?: SearchFilters) => {
    const activeFilters = searchFilters || filters;
    
    // Update URL params
    const params: Record<string, string> = {
      mode: 'advanced',
      page: page.toString(),
    };
    if (activeFilters.threadId) params.threadId = activeFilters.threadId;
    if (activeFilters.contractName) params.contract = activeFilters.contractName;
    if (activeFilters.contractVersion) params.version = activeFilters.contractVersion.toString();
    if (activeFilters.status) params.status = activeFilters.status;
    if (activeFilters.timeRange !== 'all') params.timeRange = activeFilters.timeRange;
    if (activeFilters.refs.length > 0 && activeFilters.refs[0].key) {
      params.refKey = activeFilters.refs[0].key;
      params.refValue = activeFilters.refs[0].value;
    }
    setSearchParams(params);

    setIsSearching(true);
    setError(null);
    setHasSearched(true);
    setCurrentPage(page);
    const offset = (page - 1) * resultsPerPage;

    try {
      let results: Thread[] = [];

      // Search by thread ID
      if (activeFilters.threadId) {
        const thread = await graphqlClient.getThread(activeFilters.threadId);
        results = thread ? [thread] : [];
        setTotalResults(results.length);
      }
      // Single ref search
      else if (activeFilters.refs.length > 0 && activeFilters.refs[0].key && activeFilters.refs[0].value) {
        // Search by first ref (only support one ref for now)
        const { startedAfter, startedBefore } = getTimeFilters(activeFilters);
        const response = await graphqlClient.getThreadsByRef({
          refKey: activeFilters.refs[0].key,
          refValue: activeFilters.refs[0].value,
          status: activeFilters.status,
          startedAfter,
          startedBefore,
          limit: resultsPerPage,
          offset,
        });
        results = response.threads;
        setTotalResults(response.totalCount);
      }
      // Contract search
      else if (activeFilters.contractName) {
        // Search by contract
        const { startedAfter, startedBefore } = getTimeFilters(activeFilters);
        const response = await graphqlClient.getThreadsByContract({
          contractName: activeFilters.contractName,
          contractVersion: activeFilters.contractVersion,
          status: activeFilters.status,
          startedAfter,
          startedBefore,
          limit: resultsPerPage,
          offset,
        });
        results = response.threads;
        setTotalResults(response.totalCount);
      }
      // General search with filters
      else {
        const { startedAfter, startedBefore } = getTimeFilters(activeFilters);
        const response = await graphqlClient.getThreads({
          status: activeFilters.status,
          startedAfter,
          startedBefore,
          limit: resultsPerPage,
          offset,
        });
        results = response.threads;
        setTotalResults(response.totalCount);
      }

      // Client-side filtering for multiple refs (if needed)
      if (filters.refs.length > 1) {
        results = results.filter(thread => {
          if (!thread.refs) return false;
          return filters.refs.every(refFilter => {
            if (!refFilter.key || !refFilter.value) return true;
            const threadRefs = typeof thread.refs === 'string' 
              ? JSON.parse(thread.refs) 
              : thread.refs;
            return threadRefs[refFilter.key] === refFilter.value;
          });
        });
      }

      setSearchResults(results);
    } catch (err) {
      setError((err as Error).message);
      setSearchResults([]);
    } finally {
      setIsSearching(false);
    }
  };

  const getTimeFilters = (activeFilters?: SearchFilters) => {
    const filterToUse = activeFilters || filters;
    const now = new Date();
    let startedAfter: string | undefined;
    let startedBefore: string | undefined;

    switch (filterToUse.timeRange) {
      case '24h':
        startedAfter = new Date(now.getTime() - 24 * 60 * 60 * 1000).toISOString();
        break;
      case '7d':
        startedAfter = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000).toISOString();
        break;
      case '30d':
        startedAfter = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000).toISOString();
        break;
      case 'custom':
        startedAfter = filterToUse.startedAfter;
        startedBefore = filterToUse.startedBefore;
        break;
    }

    return { startedAfter, startedBefore };
  };

  const addRefFilter = () => {
    setFilters({
      ...filters,
      refs: [...filters.refs, { key: '', value: '' }],
    });
  };

  const removeRefFilter = (index: number) => {
    setFilters({
      ...filters,
      refs: filters.refs.filter((_, i) => i !== index),
    });
  };

  const updateRefFilter = (index: number, field: 'key' | 'value', value: string) => {
    const newRefs = [...filters.refs];
    newRefs[index][field] = value;
    setFilters({ ...filters, refs: newRefs });
  };

  return (
    <AppLayout>
      <div className="p-6 max-w-7xl mx-auto">
        {/* Compact Header */}
        <div className="mb-4">
          <h1 className="text-2xl font-bold text-black">Threads</h1>
        </div>

        {/* Compact Search Mode Toggle */}
        <div className="mb-4 inline-flex rounded-lg border border-gray-200 bg-white p-1">
          <button
            onClick={() => setSearchMode('advanced')}
            className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
              searchMode === 'advanced'
                ? 'bg-gray-900 text-white shadow-sm'
                : 'text-gray-600 hover:text-gray-900'
            }`}
          >
            Advanced Search
          </button>
          <button
            onClick={() => setSearchMode('chat')}
            className={`cursor-pointer px-3 py-1.5 text-sm font-medium rounded-md transition-colors flex items-center gap-1.5 ${
              searchMode === 'chat'
                ? 'bg-gray-900 text-white shadow-sm'
                : 'text-gray-600 hover:text-gray-900'
            }`}
          >
            <Sparkles className="w-4 h-4" />
            AI Analyze
          </button>
        </div>

        {/* AI Chat Interface */}
        {searchMode === 'chat' && (
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm h-[600px] flex flex-col">
            <ThreadChat />
          </div>
        )}

        {/* Advanced Search */}
        {searchMode === 'advanced' && (
          <div>
            <AdvancedSearchFilters
              filters={filters}
              onChange={setFilters}
              onSearch={performAdvancedSearch}
              isSearching={isSearching}
              addRefFilter={addRefFilter}
              removeRefFilter={removeRefFilter}
              updateRefFilter={updateRefFilter}
            />

            <div className="mt-8">
              {isSearching && (
                <div className="flex items-center justify-center py-12">
                  <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-black"></div>
                </div>
              )}

              {error && (
                <div className="border border-red-200 rounded-lg bg-red-50 p-6 mt-4">
                  <h3 className="text-red-800 font-bold mb-2">Search Error</h3>
                  <p className="text-red-600">{error}</p>
                </div>
              )}

              {!isSearching && !error && hasSearched && (
                <div className="mt-4">
                  <ThreadSearchResults threads={searchResults} navigate={navigate} />
                  {(currentPage > 1 || totalResults > resultsPerPage) && (
                    <PaginationControls
                      currentPage={currentPage}
                      totalResults={totalResults}
                      resultsPerPage={resultsPerPage}
                      onPageChange={(page) => performAdvancedSearch(page)}
                    />
                  )}
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </AppLayout>
  );
}

// Pagination Controls Component
function PaginationControls({
  currentPage,
  totalResults,
  resultsPerPage,
  onPageChange,
}: {
  currentPage: number;
  totalResults: number;
  resultsPerPage: number;
  onPageChange: (page: number) => void;
}) {
  const totalPages = Math.ceil(totalResults / resultsPerPage);
  const hasNextPage = currentPage < totalPages;
  const hasPrevPage = currentPage > 1;
  
  const startResult = (currentPage - 1) * resultsPerPage + 1;
  const endResult = Math.min(currentPage * resultsPerPage, totalResults);

  return (
    <div className="flex items-center justify-between mt-4 px-4 py-3 bg-white border border-gray-200 rounded-lg">
      <div className="text-sm text-gray-600">
        Showing {startResult}-{endResult} of {totalResults.toLocaleString()} results
      </div>
      <div className="flex gap-2">
        <button
          onClick={() => onPageChange(currentPage - 1)}
          disabled={!hasPrevPage}
          className="px-3 py-1.5 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-1"
        >
          <ChevronLeft className="w-4 h-4" />
          Previous
        </button>
        <button
          onClick={() => onPageChange(currentPage + 1)}
          disabled={!hasNextPage}
          className="px-3 py-1.5 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-1"
        >
          Next
          <ChevronRight className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
}

// Advanced Search Filters Component
function AdvancedSearchFilters({
  filters,
  onChange,
  onSearch,
  isSearching,
  addRefFilter,
  removeRefFilter,
  updateRefFilter,
}: {
  filters: SearchFilters;
  onChange: (filters: SearchFilters) => void;
  onSearch: (page?: number) => void;
  isSearching: boolean;
  addRefFilter: () => void;
  removeRefFilter: (index: number) => void;
  updateRefFilter: (index: number, field: 'key' | 'value', value: string) => void;
}) {
  const [showAdvanced, setShowAdvanced] = useState(false);

  return (
    <div className="bg-white border border-gray-200 rounded-lg p-3 space-y-2">
      {/* Thread ID */}
      <div>
        <label className="block text-xs font-medium text-gray-700 mb-1">
          <Hash className="inline w-3 h-3 mr-1" />
          Thread ID
        </label>
        <input
          type="text"
          placeholder="Enter exact thread ID..."
          value={filters.threadId || ''}
          onChange={(e) => onChange({ ...filters, threadId: e.target.value })}
          className="w-full px-2.5 py-1.5 text-xs border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-900"
        />
      </div>

      {/* Contract */}
      <div className="flex gap-2">
        <div className="flex-1">
          <label className="block text-xs font-medium text-gray-700 mb-1">
            <FileText className="inline w-3 h-3 mr-1" />
            Contract Name
          </label>
          <input
            type="text"
            placeholder="e.g., order_fulfillment"
            value={filters.contractName || ''}
            onChange={(e) => onChange({ ...filters, contractName: e.target.value })}
            className="w-full px-2.5 py-1.5 text-xs border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-900"
          />
        </div>
        <div className="w-20">
          <label className="block text-xs font-medium text-gray-700 mb-1">Version</label>
          <input
            type="number"
            placeholder="1"
            value={filters.contractVersion || ''}
            onChange={(e) => onChange({ ...filters, contractVersion: e.target.value ? parseInt(e.target.value) : undefined })}
            className="w-full px-2.5 py-1.5 text-xs border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-900 text-center"
          />
        </div>
      </div>

      {/* Refs */}
      <div>
        <label className="block text-xs font-medium text-gray-700 mb-1">References</label>
        {filters.refs.map((ref, idx) => (
          <div key={idx} className="flex gap-1.5 mb-1.5">
            <input
              placeholder="Key"
              value={ref.key}
              onChange={(e) => updateRefFilter(idx, 'key', e.target.value)}
              className="w-32 px-2.5 py-1.5 text-xs border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-900"
            />
            <input
              placeholder="Value"
              value={ref.value}
              onChange={(e) => updateRefFilter(idx, 'value', e.target.value)}
              className="flex-1 px-2.5 py-1.5 text-xs border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-900"
            />
            <button
              onClick={() => removeRefFilter(idx)}
              className="px-1.5 py-1.5 text-red-600 hover:bg-red-50 rounded-md transition-colors"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
        ))}
        <button
          onClick={addRefFilter}
          className="text-xs text-gray-600 hover:text-gray-900 font-medium mt-0.5"
        >
          + Add Reference
        </button>
      </div>

      {/* Advanced Filters Toggle */}
      <button
        onClick={() => setShowAdvanced(!showAdvanced)}
        className="flex items-center gap-1 text-xs font-medium text-gray-600 hover:text-gray-900 pt-1"
      >
        {showAdvanced ? <ChevronUp className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
        {showAdvanced ? 'Hide' : 'Show'} Advanced
      </button>

      {/* Advanced Filters */}
      {showAdvanced && (
        <div className="space-y-2 pt-2 border-t border-gray-200">
          {/* Time Range */}
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">
              <Calendar className="inline w-3 h-3 mr-1" />
              Time Range
            </label>
            <select
              value={filters.timeRange}
              onChange={(e) => onChange({ ...filters, timeRange: e.target.value as any })}
              className="w-full px-2.5 py-1.5 text-xs border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-900"
            >
              <option value="all">All Time</option>
              <option value="24h">Last 24 Hours</option>
              <option value="7d">Last 7 Days</option>
              <option value="30d">Last 30 Days</option>
              <option value="custom">Custom Range</option>
            </select>
          </div>

          {/* Custom Date Range */}
          {filters.timeRange === 'custom' && (
            <div className="flex gap-2">
              <div className="flex-1">
                <label className="block text-xs font-medium text-gray-700 mb-1">Start (From)</label>
                <input
                  type="datetime-local"
                  value={filters.startedAfter || ''}
                  onChange={(e) => onChange({ ...filters, startedAfter: e.target.value })}
                  className="w-full px-2 py-1 text-xs border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-900"
                />
              </div>
              <div className="flex-1">
                <label className="block text-xs font-medium text-gray-700 mb-1">End (To)</label>
                <input
                  type="datetime-local"
                  value={filters.startedBefore || ''}
                  onChange={(e) => onChange({ ...filters, startedBefore: e.target.value })}
                  className="w-full px-2 py-1 text-xs border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-900"
                />
              </div>
            </div>
          )}

          {/* Status Filter */}
          <div>
            <label className="block text-xs font-medium text-gray-700 mb-1">Status</label>
            <select
              value={filters.status || ''}
              onChange={(e) => onChange({ ...filters, status: e.target.value || undefined })}
              className="w-full px-2.5 py-1.5 text-xs border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-gray-900"
            >
              <option value="">All Statuses</option>
              <option value="active">Active</option>
              <option value="completed">Completed</option>
              <option value="failed">Failed</option>
              <option value="cancelled">Cancelled</option>
            </select>
          </div>
        </div>
      )}

      {/* Search Button */}
      <button
        onClick={() => onSearch(1)}
        disabled={isSearching}
        className="w-full py-1.5 px-4 text-xs bg-gray-900 text-white rounded-md font-medium hover:bg-gray-800 disabled:bg-gray-300 disabled:cursor-not-allowed transition-colors mt-1"
      >
        {isSearching ? 'Searching...' : 'Search Threads'}
      </button>
    </div>
  );
}

// Search Results Component
function ThreadSearchResults({ threads, navigate }: { threads: Thread[]; navigate: any }) {
  if (threads.length === 0) {
    return (
      <div className="border border-gray-200 rounded-lg p-8 text-center bg-white">
        <Search className="w-12 h-12 text-gray-400 mx-auto mb-3" />
        <h3 className="text-lg font-semibold text-gray-900 mb-1">No threads found</h3>
        <p className="text-sm text-gray-600">Try adjusting your search criteria</p>
      </div>
    );
  }

  const getStatusBadge = (status: string) => {
    const baseClasses = 'text-xs px-1.5 py-0.5 rounded-md font-medium';
    const configs = {
      active: `${baseClasses} bg-blue-50 text-blue-700`,
      completed: `${baseClasses} bg-green-50 text-green-700`,
      failed: `${baseClasses} bg-red-50 text-red-700`,
      cancelled: `${baseClasses} bg-gray-100 text-gray-700`,
    };
    return configs[status as keyof typeof configs] || `${baseClasses} bg-gray-100 text-gray-700`;
  };

  return (
    <div className="space-y-2">
      <div className="mb-3 text-xs text-gray-600">
        Found <span className="font-semibold text-gray-900">{threads.length}</span> thread{threads.length !== 1 ? 's' : ''}
      </div>

      {threads.map((thread) => {
        const refs = thread.refs 
          ? (typeof thread.refs === 'string' ? JSON.parse(thread.refs) : thread.refs)
          : {};

        return (
          <div
            key={thread.id}
            onClick={() => navigate(`/u/threads/${thread.id}`)}
            className="bg-white border border-gray-200 rounded-lg p-3 hover:shadow-sm hover:border-gray-300 transition-all cursor-pointer"
          >
            <div className="flex justify-between items-start">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1.5">
                  <h3 className="font-semibold text-sm text-gray-900 truncate">{thread.id}</h3>
                  <span className={getStatusBadge(thread.status)}>{thread.status}</span>
                </div>

                <p className="text-xs text-gray-500 font-mono mb-2 truncate">
                  {thread.contractName && thread.contractVersion ? `${thread.contractName} v${thread.contractVersion}` : thread.contractName || '-'}
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
                      {Object.entries(refs).slice(0, 2).map(([key, value]) => (
                        <span
                          key={key}
                          className="inline-flex items-center px-1.5 py-0.5 bg-blue-50 text-blue-700 text-xs rounded"
                        >
                          <span className="font-medium">{key}:</span>
                          <span className="ml-0.5">{String(value)}</span>
                        </span>
                      ))}
                      {Object.keys(refs).length > 2 && (
                        <span className="text-xs text-gray-500">+{Object.keys(refs).length - 2} more</span>
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

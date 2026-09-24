import { useNavigate, useSearchParams } from 'react-router';
import { useState, useEffect } from 'react';
import { Search, Filter, ChevronDown, ChevronUp, X, Calendar, Hash, FileText, ChevronLeft, ChevronRight, PlusCircle } from 'lucide-react';
import WorkspacePage from '~/components/WorkspacePage';
import { graphqlClient, type Thread } from '~/lib/graphql';
import { api } from '~/lib/api';
import { formatDistanceToNow } from 'date-fns';



interface RefFilter {
  key: string;
  value: string;
}

interface SearchFilters {
  searchQuery?: string; // Simple search input
  threadId?: string;
  contractName?: string;
  contractVersion?: number;
  refs: RefFilter[];
  timeRange: 'all' | '24h' | '7d' | '30d' | 'custom';
  startedAfter?: string;
  startedBefore?: string;
  status?: string;
}

type ParsedSearchType = 'thread_id' | 'contract' | 'reference' | 'tag';

interface ParsedSearch {
  type: ParsedSearchType;
  value: string;
}

export default function ThreadsPage() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  
  const [filters, setFilters] = useState<SearchFilters>({
    searchQuery: '',
    refs: [{ key: '', value: '' }],
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
    const page = searchParams.get('page');
    const searchQuery = searchParams.get('q');
    const contractName = searchParams.get('contract');
    const contractVersion = searchParams.get('version');
    const threadId = searchParams.get('threadId');
    const status = searchParams.get('status');
    const timeRange = searchParams.get('timeRange');
    const refKey = searchParams.get('refKey');
    const refValue = searchParams.get('refValue');

    if (page) setCurrentPage(parseInt(page));

    const restoredFilters: SearchFilters = {
      searchQuery: searchQuery || '',
      refs: refKey && refValue ? [{ key: refKey, value: refValue }] : [],
      timeRange: (timeRange as any) || 'all',
      threadId: threadId || undefined,
      contractName: contractName || undefined,
      contractVersion: contractVersion ? parseInt(contractVersion) : undefined,
      status: status || undefined,
    };
    setFilters(restoredFilters);
  }, []);

  // Re-execute search when URL params change (after initial mount)
  useEffect(() => {
    const page = parseInt(searchParams.get('page') || '1');
    performAdvancedSearch(page);
  }, [searchParams]);

  const parseSearchQuery = (query: string): ParsedSearch => {
    const trimmed = query.trim();
    
    // Check for explicit prefixes
    if (trimmed.startsWith('tag:')) {
      return {
        type: 'tag',
        value: trimmed.substring(4).trim(),
      };
    }

    if (trimmed.startsWith('contract:')) {
      return {
        type: 'contract',
        value: trimmed.substring(9).trim(),
      };
    }
    
    if (trimmed.startsWith('ref:')) {
      return {
        type: 'reference',
        value: trimmed.substring(4).trim(),
      };
    }
    
    if (trimmed.startsWith('thread:')) {
      return {
        type: 'thread_id',
        value: trimmed.substring(7).trim(),
      };
    }
    
    // Auto-detect: UUID = thread_id, otherwise = reference
    const uuidRegex = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    
    if (uuidRegex.test(trimmed)) {
      return { type: 'thread_id', value: trimmed };
    }
    
    // Default to reference search
    return { type: 'reference', value: trimmed };
  };

  const performAdvancedSearch = async (page: number = 1, searchFilters?: SearchFilters) => {
    const activeFilters = searchFilters || filters;
    
    // Update URL params
    const params: Record<string, string> = {
      mode: 'advanced',
      page: page.toString(),
    };
    if (activeFilters.searchQuery) params.q = activeFilters.searchQuery;
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
      const { startedAfter, startedBefore } = getTimeFilters(activeFilters);

      // Priority 1: Simple search query (if provided)
      if (activeFilters.searchQuery && activeFilters.searchQuery.trim()) {
        const parsed = parseSearchQuery(activeFilters.searchQuery);
        
        switch (parsed.type) {
          case 'thread_id':
            // Exact thread ID lookup
            const thread = await graphqlClient.getThread(parsed.value);
            results = thread ? [thread] : [];
            setTotalResults(results.length);
            break;
            
          case 'contract':
            // Search by contract name
            const contractResponse = await graphqlClient.getThreadsByContract({
              contractName: parsed.value,
              contractVersion: activeFilters.contractVersion,
              status: activeFilters.status,
              startedAfter,
              startedBefore,
              limit: resultsPerPage,
              offset,
            });
            results = contractResponse.threads;
            setTotalResults(contractResponse.totalCount);
            break;
            
          case 'tag':
            // Search by tag
            const tagResponse = await graphqlClient.getThreads({
              tags: [parsed.value],
              contractName: activeFilters.contractName,
              status: activeFilters.status,
              startedAfter,
              startedBefore,
              limit: resultsPerPage,
              offset,
            });
            results = tagResponse.threads;
            setTotalResults(tagResponse.totalCount);
            break;
            
          case 'reference':
            // Search by reference value (omit refKey to search across all ref keys)
            const refResponse = await graphqlClient.getThreadsByRef({
              // refKey omitted - searches across all reference keys
              refValue: parsed.value,
              status: activeFilters.status,
              startedAfter,
              startedBefore,
              limit: resultsPerPage,
              offset,
            });
            results = refResponse.threads;
            setTotalResults(refResponse.totalCount);
            
            // Apply advanced contract filter if provided
            if (activeFilters.contractName) {
              results = results.filter(t => t.contractName === activeFilters.contractName);
              setTotalResults(results.length);
            }
            break;
        }
      }
      // Priority 2: Advanced filters (legacy behavior)
      else if (activeFilters.threadId) {
        const thread = await graphqlClient.getThread(activeFilters.threadId);
        results = thread ? [thread] : [];
        setTotalResults(results.length);
      }
      else if (activeFilters.refs.length > 0 && activeFilters.refs[0].key && activeFilters.refs[0].value) {
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
      else if (activeFilters.contractName) {
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
      else {
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
    <WorkspacePage eyebrow="Workspace / Activity" title="Threads" description="Find and inspect workflow execution across your workspace.">
      <div className="space-y-5">
        {/* Advanced Search */}
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

            <div className="mt-6">
              {isSearching && (
                <div className="flex items-center justify-center py-12">
                  <div className="h-8 w-8 animate-spin rounded-full border-2 border-stone-200 border-t-emerald-600"></div>
                </div>
              )}

              {error && (
                <div className="mt-4 rounded-2xl border border-red-200 bg-red-50 p-6">
                  <h3 className="text-red-800 font-bold mb-2">Search Error</h3>
                  <p className="text-red-600">{error}</p>
                </div>
              )}

              {!isSearching && !error && hasSearched && (
                <div className="mt-4">
                  <ThreadSearchResults 
                    threads={searchResults} 
                    navigate={navigate} 
                    onTagClick={(tag) => {
                      const newFilters = { ...filters, searchQuery: `tag:${tag}` };
                      setFilters(newFilters);
                      performAdvancedSearch(1, newFilters);
                    }}
                  />
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
      </div>
    </WorkspacePage>
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
    <div className="mt-5 flex flex-col gap-3 rounded-2xl border border-stone-200 bg-white px-5 py-4 shadow-sm sm:flex-row sm:items-center sm:justify-between">
      <div className="text-sm text-gray-600 text-center sm:text-left">
        Showing {startResult}-{endResult} of {totalResults.toLocaleString()} results
      </div>
      <div className="grid grid-cols-2 gap-2 sm:flex">
        <button
          onClick={() => onPageChange(currentPage - 1)}
          disabled={!hasPrevPage}
          className="flex items-center justify-center gap-1 rounded-lg border border-stone-200 bg-white px-3 py-2 text-sm font-medium text-stone-700 transition-colors hover:bg-stone-50 disabled:cursor-not-allowed disabled:opacity-50"
        >
          <ChevronLeft className="w-4 h-4" />
          Previous
        </button>
        <button
          onClick={() => onPageChange(currentPage + 1)}
          disabled={!hasNextPage}
          className="flex items-center justify-center gap-1 rounded-lg border border-stone-200 bg-white px-3 py-2 text-sm font-medium text-stone-700 transition-colors hover:bg-stone-50 disabled:cursor-not-allowed disabled:opacity-50"
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

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !isSearching) {
      onSearch(1);
    }
  };

  return (
    <div className="space-y-5 rounded-2xl border border-stone-200 bg-white p-5 shadow-sm sm:p-6">
      {/* Simple Search Box */}
      <div>
        <label className="mb-2 block text-sm font-semibold text-stone-800">
          <Search className="inline w-4 h-4 mr-1" />
          Search Threads
        </label>
        <div className="flex flex-col gap-2 sm:flex-row">
          <input
            type="text"
            placeholder="Search by thread ID, contract, tag, or reference value..."
            value={filters.searchQuery || ''}
            onChange={(e) => onChange({ ...filters, searchQuery: e.target.value })}
            onKeyPress={handleKeyPress}
            className="min-w-0 flex-1 rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
          />
          <button
            onClick={() => onSearch(1)}
            disabled={isSearching}
            className="inline-flex items-center justify-center gap-2 rounded-lg bg-stone-900 px-6 py-3 text-sm font-medium text-white transition-colors hover:bg-stone-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isSearching ? (
              <>
                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>
                Searching...
              </>
            ) : (
              <>
                <Search className="w-4 h-4" />
                Search
              </>
            )}
          </button>
        </div>
        <p className="text-xs text-gray-500 mt-1.5 break-words">
          <b>Tip</b>: Use <code className="bg-gray-100 px-1 py-0.5 rounded break-all">contract:order_fulfillment</code>,{' '}
          <code className="bg-gray-100 px-1 py-0.5 rounded break-all">ref:customer@example.com</code> or{' '}
          <code className="bg-gray-100 px-1 py-0.5 rounded break-all">tag:vip</code> for specific searches
        </p>
      </div>

      {/* Advanced Filters Toggle */}
      <button
        onClick={() => setShowAdvanced(!showAdvanced)}
        className="flex items-center gap-1.5 border-t border-stone-100 pt-4 text-sm font-medium text-stone-600 hover:text-emerald-700"
      >
        {showAdvanced ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        {showAdvanced ? 'Hide' : 'Show'} Advanced Filters
      </button>

      {/* Advanced Filters */}
      {showAdvanced && (
        <div className="grid gap-5 border-t border-stone-100 pt-5 md:grid-cols-2">
          {/* Contract Filter (for ref: searches) */}
          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">
              <FileText className="inline w-4 h-4 mr-1" />
              Filter by Contract
            </label>
            <input
              type="text"
              placeholder="e.g., order_fulfillment (optional)"
              value={filters.contractName || ''}
              onChange={(e) => onChange({ ...filters, contractName: e.target.value })}
              className="w-full rounded-lg border border-stone-200 bg-white px-3 py-2.5 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
            />
          </div>

          {/* Time Range */}
          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">
              <Calendar className="inline w-4 h-4 mr-1" />
              Time Range
            </label>
            <select
              value={filters.timeRange}
              onChange={(e) => onChange({ ...filters, timeRange: e.target.value as any })}
              className="w-full rounded-lg border border-stone-200 bg-white px-3 py-2.5 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
            <div className="flex flex-col gap-2 sm:flex-row">
              <div className="flex-1">
                <label className="mb-2 block text-sm font-medium text-stone-700">Start (From)</label>
                <input
                  type="datetime-local"
                  value={filters.startedAfter || ''}
                  onChange={(e) => onChange({ ...filters, startedAfter: e.target.value })}
                  className="w-full rounded-lg border border-stone-200 bg-white px-3 py-2.5 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
                />
              </div>
              <div className="flex-1">
                <label className="mb-2 block text-sm font-medium text-stone-700">End (To)</label>
                <input
                  type="datetime-local"
                  value={filters.startedBefore || ''}
                  onChange={(e) => onChange({ ...filters, startedBefore: e.target.value })}
                  className="w-full rounded-lg border border-stone-200 bg-white px-3 py-2.5 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
                />
              </div>
            </div>
          )}

          {/* Ref Key/Value Filter */}
          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">
              <Hash className="inline w-4 h-4 mr-1" />
              Filter by Reference
            </label>
            <div className="space-y-2">
              {filters.refs.map((ref, index) => (
                <div key={index} className="flex flex-col gap-2 sm:flex-row">
                  <input
                    type="text"
                    placeholder="Ref key (e.g., customer)"
                    value={ref.key}
                    onChange={(e) => updateRefFilter(index, 'key', e.target.value)}
                    className="w-full rounded-lg border border-stone-200 bg-white px-3 py-2.5 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
                  />
                  <input
                    type="text"
                    placeholder="Ref value (e.g., customer@example.com)"
                    value={ref.value}
                    onChange={(e) => updateRefFilter(index, 'value', e.target.value)}
                    className="w-full rounded-lg border border-stone-200 bg-white px-3 py-2.5 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
                  />
                  <div className="flex justify-end gap-1 sm:justify-start">
                    {filters.refs.length > 1 && (
                      <button
                        type="button"
                        onClick={() => removeRefFilter(index)}
                        className="p-2 text-red-700 hover:text-red-800 transition-colors"
                        title="Remove reference"
                      >
                        <X className="w-4 h-4" />
                      </button>
                    )}
                    {index === filters.refs.length - 1 && (
                      <button
                        type="button"
                        onClick={addRefFilter}
                        className="p-2 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded-md transition-colors"
                        title="Add another reference"
                      >
                        <PlusCircle className="w-4 h-4" />
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
            <p className="text-xs text-gray-500 mt-1">
              Search threads by external reference (e.g., payment ID, customer email)
            </p>
          </div>

          {/* Status Filter */}
          <div>
            <label className="mb-2 block text-sm font-medium text-stone-700">Status</label>
            <select
              value={filters.status || ''}
              onChange={(e) => onChange({ ...filters, status: e.target.value || undefined })}
              className="w-full rounded-lg border border-stone-200 bg-white px-3 py-2.5 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100"
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
    </div>
  );
}

// Search Results Component
function ThreadSearchResults({ 
  threads, 
  navigate,
  onTagClick 
}: { 
  threads: Thread[]; 
  navigate: any;
  onTagClick?: (tag: string) => void;
}) {
  if (threads.length === 0) {
    return (
      <div className="rounded-2xl border border-stone-200 bg-white p-12 text-center shadow-sm">
        <Search className="mx-auto mb-4 h-12 w-12 rounded-xl bg-emerald-50 p-2.5 text-emerald-700" />
        <h3 className="text-lg font-semibold text-gray-900 mb-1">No threads found</h3>
        <p className="text-sm text-gray-600">Try adjusting your search criteria</p>
      </div>
    );
  }

  const getStatusBadge = (status: string) => {
    const baseClasses = 'rounded-full px-2.5 py-1 text-xs font-medium capitalize';
    const configs = {
      active: `${baseClasses} bg-amber-50 text-amber-700`,
      completed: `${baseClasses} bg-emerald-50 text-emerald-700`,
      failed: `${baseClasses} bg-red-50 text-red-700`,
      cancelled: `${baseClasses} bg-gray-100 text-gray-700`,
    };
    return configs[status as keyof typeof configs] || `${baseClasses} bg-gray-100 text-gray-700`;
  };

  return (
    <div className="space-y-2">
      <div className="mb-3 text-xs font-medium uppercase tracking-wide text-stone-500">
        Found <span className="font-semibold text-gray-900">{threads.length}</span> thread{threads.length !== 1 ? 's' : ''}
      </div>

      {threads.map((thread) => {
        const refs = thread.refs 
          ? (typeof thread.refs === 'string' ? JSON.parse(thread.refs) : thread.refs)
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
            role="button"
            tabIndex={0}
            aria-label={`Open thread ${threadTitle}`}
            onClick={() => navigate(`/u/threads/${thread.id}`)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                navigate(`/u/threads/${thread.id}`);
              }
            }}
            className="cursor-pointer overflow-hidden rounded-2xl border border-stone-200 bg-white p-5 shadow-sm transition-all hover:border-emerald-200 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-600"
          >
            <div className="flex flex-col gap-3 sm:flex-row sm:justify-between sm:items-start">
              <div className="flex-1 min-w-0">
                <div className="flex flex-wrap items-start gap-2 mb-1.5">
                  <h3 className="min-w-0 flex-1 break-words text-base font-semibold text-stone-900 sm:truncate">
                    {threadTitle}
                  </h3>
                  <span className={getStatusBadge(thread.status)}>{thread.status}</span>
                </div>

                {thread.contractName && thread.contractVersion ? (
                  <p className="text-xs text-gray-500 font-mono mb-2 truncate">
                    {`${thread.contractName} v${thread.contractVersion}`}
                  </p>
                ) : (
                  <div className="flex items-center gap-1 mb-2">
                    {thread.tags && thread.tags.length > 0 && (
                      <>
                        {thread.tags.slice(0, 3).map((tag) => (
                          <span
                            key={tag}
                            onClick={(e) => {
                              if (onTagClick) {
                                e.stopPropagation();
                                onTagClick(tag);
                              }
                            }}
                            className={`px-1.5 py-0.5 rounded-full text-[10px] font-medium bg-violet-50 text-violet-700 border border-violet-200 ${onTagClick ? 'cursor-pointer hover:bg-violet-100' : ''}`}
                          >
                            {tag}
                          </span>
                        ))}
                        {thread.tags.length > 3 && (
                          <span className="text-[10px] text-gray-500">+{thread.tags.length - 3}</span>
                        )}
                      </>
                    )}
                  </div>
                )}

                <div className="flex flex-col items-start gap-2 text-xs text-gray-600 sm:flex-row sm:flex-wrap sm:items-center sm:gap-3">
                  {thread.startedAt && (
                    <div className="flex items-center gap-1 shrink-0">
                      <Calendar className="w-3 h-3 shrink-0" />
                      {formatDistanceToNow(new Date(thread.startedAt), { addSuffix: true })}
                    </div>
                  )}
                  {Object.keys(refs).length > 0 && (
                    <div className="flex max-w-full flex-wrap items-center gap-1.5">
                      {Object.entries(refs).slice(0, 2).map(([key, value]) => (
                        <span
                          key={key}
                          className="inline-flex min-w-0 max-w-full items-center px-1.5 py-0.5 bg-gray-50 border border-gray-200 text-xs rounded"
                        >
                          <span className="shrink-0 font-medium text-gray-500">{key}:</span>
                          <span className="ml-0.5 min-w-0 truncate font-mono text-gray-700">{String(value)}</span>
                        </span>
                      ))}
                      {Object.keys(refs).length > 2 && (
                        <span className="text-xs text-gray-500">+{Object.keys(refs).length - 2} more</span>
                      )}
                    </div>
                  )}
                  {/* Tags for threads with a contract are displayed next to the contract */}
                  {thread.contractName && thread.tags && thread.tags.length > 0 && (
                    <div className="flex max-w-full flex-wrap items-center gap-1">
                      {thread.tags.slice(0, 3).map((tag) => (
                        <span
                          key={tag}
                          onClick={(e) => {
                            if (onTagClick) {
                              e.stopPropagation();
                              onTagClick(tag);
                            }
                          }}
                          className={`px-1.5 py-0.5 rounded-full text-[10px] font-medium bg-violet-50 text-violet-700 border border-violet-200 ${onTagClick ? 'cursor-pointer hover:bg-violet-100' : ''}`}
                        >
                          {tag}
                        </span>
                      ))}
                      {thread.tags.length > 3 && (
                        <span className="text-[10px] text-gray-500">+{thread.tags.length - 3}</span>
                      )}
                    </div>
                  )}
                </div>
              </div>

              <div className="flex-shrink-0 self-stretch sm:self-auto">
                <button className="w-full rounded-lg px-3 py-1.5 text-right text-xs font-semibold text-emerald-700 transition-colors hover:bg-emerald-50 hover:text-emerald-900 sm:w-auto sm:text-left">
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

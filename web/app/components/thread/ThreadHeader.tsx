import { useState } from 'react';
import { Link } from '@remix-run/react';
import { useQuery } from '@tanstack/react-query';
import { formatDistanceToNow } from 'date-fns';
import {
  Copy,
  Check,
  Shield,
  ShieldAlert,
  Loader2,
  AlertTriangle,
  ExternalLink,
} from 'lucide-react';
import { graphqlClient, type Thread } from '~/lib/graphql';

export function ThreadHeader({ thread }: { thread: Thread }) {
  const [copied, setCopied] = useState(false);
  const [copiedRef, setCopiedRef] = useState<string | null>(null);
  const steps = thread.steps || [];
  
  // Count step statuses in a single pass
  const { successCount, failedCount, violatedCount, pendingCount } = steps.reduce(
    (acc, step) => {
      switch (step.status) {
        case 'success':
          acc.successCount++;
          break;
        case 'failed':
          acc.failedCount++;
          break;
        case 'violated':
          acc.violatedCount++;
          break;
        case 'pending':
        case 'in_progress':
          acc.pendingCount++;
          break;
      }
      return acc;
    },
    { successCount: 0, failedCount: 0, violatedCount: 0, pendingCount: 0 }
  );

  // Fetch thread hash chain verification
  const { data: hashChainStatus, isLoading: isVerifying } = useQuery({
    queryKey: ['threadIntegrity', thread.id],
    queryFn: () => graphqlClient.verifyThreadIntegrity(thread.id),
    enabled: !!thread.id && steps.length > 0,
    refetchInterval: false,
  });

  // Resolve owner name from ownerId
  const { data: resolvedOwner } = useQuery({
    queryKey: ['resolveOwner', thread.ownerId],
    queryFn: () => graphqlClient.resolveActors([thread.ownerId]),
    enabled: !!thread.ownerId,
  });

  const copyThreadId = () => {
    navigator.clipboard.writeText(thread.id);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const statusConfig = {
    active: { label: 'Active', color: 'bg-blue-50 text-blue-700 border-blue-200' },
    completed: { label: 'Completed', color: 'bg-green-50 text-green-700 border-green-200' },
    failed: { label: 'Failed', color: 'bg-red-50 text-red-700 border-red-200' },
    cancelled: { label: 'Cancelled', color: 'bg-gray-100 text-gray-700 border-gray-300' },
  };

  const config = statusConfig[thread.status as keyof typeof statusConfig] || statusConfig.active;

  return (
    <div className="border-b border-gray-200 pb-6">
      {/* Thread Title and Status */}
      <div className="flex items-center gap-3 mb-4">
        <h1 className="text-2xl font-semibold text-gray-900">
          {thread.label
            ? `${thread.label} (${thread.id.split('-').pop()})`
            : thread.contractName
              ? `${thread.contractName} (${thread.id.split('-').pop()})`
              : thread.id}
        </h1>
        <button
          onClick={copyThreadId}
          className="p-1.5 hover:bg-gray-100 rounded-lg transition-colors"
          title="Copy thread ID"
        >
          {copied ? <Check className="w-4 h-4 text-green-600" /> : <Copy className="w-4 h-4 text-gray-400" />}
        </button>
        <span className={`px-2.5 py-1 rounded-lg text-xs font-medium border ${config.color}`}>
          {config.label}
        </span>
        
        {/* Hash Chain Verification Badge */}
        {steps.length > 0 && (
          <div className="flex items-center">
            {isVerifying ? (
              <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-gray-50 border border-gray-200">
                <Loader2 className="w-3.5 h-3.5 text-gray-400 animate-spin" />
                <span className="text-xs text-gray-600">Verifying...</span>
              </div>
            ) : hashChainStatus ? (
              <div 
                className={`flex items-center gap-1.5 px-2.5 py-1 rounded-lg border cursor-help ${
                  hashChainStatus.verified 
                    ? 'bg-green-50 border-green-200' 
                    : 'bg-red-50 border-red-200'
                }`}
                title={hashChainStatus.verified 
                  ? `All ${hashChainStatus.totalEvents} steps verified - No tampering detected. Verified ${formatDistanceToNow(new Date(hashChainStatus.lastVerifiedAt), { addSuffix: true })}.`
                  : `Integrity check failed - Steps may be out of order or modified. ${hashChainStatus.error || ''}`}
              >
                {hashChainStatus.verified ? (
                  <>
                    <Shield className="w-3.5 h-3.5 text-green-600" />
                    <span className="text-xs font-medium text-green-700">Hash Verified</span>
                  </>
                ) : (
                  <>
                    <ShieldAlert className="w-3.5 h-3.5 text-red-600" />
                    <span className="text-xs font-medium text-red-700">Hash Broken</span>
                  </>
                )}
              </div>
            ) : null}
          </div>
        )}
      </div>

      {/* Metadata Row */}
      <div className="flex items-center gap-6 text-sm text-gray-600 flex-wrap">
        {thread.tags && thread.tags.length > 0 && (
          <div className="flex items-center gap-1.5 flex-wrap">
            {thread.tags.map((tag) => (
              <span
                key={tag}
                className="px-2 py-0.5 rounded-full text-xs font-medium bg-violet-50 text-violet-700 border border-violet-200"
              >
                {tag}
              </span>
            ))}
          </div>
        )}
        
        {thread.startedAt && (
          <div className="flex items-center gap-2">
            <span className="text-gray-500">Started</span>
            <span className="font-medium text-gray-900">{formatDistanceToNow(new Date(thread.startedAt), { addSuffix: true })}</span>
          </div>
        )}
        
        {thread.ownerId && (
          <div className="flex items-center gap-2">
            <span className="text-gray-500">Created By</span>
            <span className="font-medium text-gray-900">
              {resolvedOwner?.[0]?.name || thread.ownerId}
            </span>
          </div>
        )}
        
        {thread.contractId && thread.contractName && thread.contractVersion && (
          <div className="flex items-center gap-2">
            <span className="text-gray-500">Contract</span>
            <Link 
              to={`/u/contracts/${thread.contractId}/versions/${thread.contractVersion}`}
              className="font-medium text-gray-900 hover:text-gray-700 hover:underline flex items-center gap-1.5 transition-colors"
            >
              {thread.contractName}
              <span className="text-gray-400">v{thread.contractVersion}</span>
              <ExternalLink className="w-3.5 h-3.5 text-gray-400" />
            </Link>
          </div>
        )}

        <div className="flex items-center gap-4 ml-auto">
          <div className="flex items-center gap-1.5">
            <span className="font-medium text-gray-900">{steps.length}</span>
            <span className="text-gray-500">Step(s)</span>
            
          </div>
          <div className="flex items-center gap-1.5">
            <div className="w-2 h-2 rounded-full bg-green-500"></div>
            <span className="font-medium text-gray-900">{successCount}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <div className="w-2 h-2 rounded-full bg-red-500"></div>
            <span className="font-medium text-gray-900">{failedCount}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <div className="w-2 h-2 rounded-full bg-orange-500"></div>
            <span className="font-medium text-gray-900">{violatedCount}</span>
          </div>
          <div className="flex items-center gap-1.5">
            <div className="w-2 h-2 rounded-full bg-gray-500"></div>
            <span className="font-medium text-gray-900">{pendingCount}</span>
          </div>
        </div>
      </div>

      {/* External References */}
      <div className="mt-4 pt-4 border-t border-gray-100">
        {(() => {
          let refsObj: Record<string, any> | null = null;
          
          if (thread.refs) {
            // Parse refs if it's a JSON string
            if (typeof thread.refs === 'string') {
              try {
                refsObj = JSON.parse(thread.refs);
              } catch (e) {
                refsObj = null;
              }
            } else if (typeof thread.refs === 'object') {
              refsObj = thread.refs;
            }
          }
          
          const refEntries = refsObj ? Object.entries(refsObj).filter(([key]) => isNaN(Number(key))) : [];
          const hasRefs = refEntries.length > 0;
          
          const copyToClipboard = (value: string, refKey: string) => {
            navigator.clipboard.writeText(value);
            setCopiedRef(refKey);
            setTimeout(() => setCopiedRef(null), 2000);
          };
          
          return hasRefs ? (
            <>
              <div className="text-xs font-semibold text-gray-700 mb-2 flex items-center gap-2">
                <span>External References</span>
                <span className="text-xs bg-gray-200 text-gray-700 px-1.5 py-0.5 rounded-full">{refEntries.length}</span>
              </div>
              <div className="flex flex-wrap gap-1">
                {refEntries.map(([key, value]) => (
                  <div
                    key={key}
                    className="inline-flex items-center gap-1 px-1.5 py-0.5 bg-gray-50 border border-gray-200 rounded text-2xs"
                  >
                    <span className="font-medium text-gray-500 text-2xs">{key}:</span>
                    <span className="text-gray-700 font-mono text-2xs truncate max-w-[100px]" title={String(value)}>
                      {String(value)}
                    </span>
                    <button
                      onClick={() => copyToClipboard(String(value), key)}
                      className="ml-0.5 p-0.5 hover:bg-gray-200 rounded transition-colors flex-shrink-0"
                      title="Copy value"
                    >
                      {copiedRef === key ? (
                        <Check className="w-2.5 h-2.5 text-green-600" />
                      ) : (
                        <Copy className="w-2.5 h-2.5 text-gray-400 hover:text-gray-600" />
                      )}
                    </button>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <p className="text-sm text-gray-400">No external system referenced</p>
          );
        })()}
      </div>

      {thread.error && (
        <div className="mt-4 flex items-start gap-2 text-sm text-red-600 bg-red-50 border border-red-200 rounded-md p-3">
          <AlertTriangle className="w-4 h-4 mt-0.5 flex-shrink-0" />
          <span>{thread.error}</span>
        </div>
      )}
    </div>
  );
}

import { formatDistanceToNow } from 'date-fns';
import {
  XCircle,
  AlertTriangle,
  Info,
  Hash,
} from 'lucide-react';
import type { ThreadNotification } from '~/lib/graphql';

export function NotificationDetailView({ notification }: { notification: ThreadNotification }) {
  const getSeverityConfig = () => {
    switch (notification.severity) {
      case 'critical':
        return {
          bg: 'bg-red-50',
          border: 'border-red-200',
          icon: <XCircle className="w-5 h-5 text-red-600" />,
          badge: 'bg-red-100 text-red-700',
        };
      case 'warning':
        return {
          bg: 'bg-yellow-50',
          border: 'border-yellow-200',
          icon: <AlertTriangle className="w-5 h-5 text-yellow-600" />,
          badge: 'bg-yellow-100 text-yellow-700',
        };
      default:
        return {
          bg: 'bg-blue-50',
          border: 'border-blue-200',
          icon: <Info className="w-5 h-5 text-blue-600" />,
          badge: 'bg-blue-100 text-blue-700',
        };
    }
  };
  const config = getSeverityConfig();

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className={`${config.bg} border ${config.border} rounded-lg p-4`}>
        <div className="flex items-center gap-2 mb-3">
          <h2 className="text-lg font-semibold text-gray-900">{notification.stepName}</h2>
          <div className="flex-shrink-0">
            {config.icon}
          </div>
          <span className={`inline-block px-2 py-1 rounded-full text-xs font-medium ${config.badge}`}>
            {notification.severity?.toUpperCase()}
          </span>
        </div>
        <p className="text-sm text-gray-700">{notification.message}</p>
      </div>

      {/* Metadata */}
      <div className="space-y-4">
        <div>
          <h3 className="text-sm font-semibold text-gray-900 mb-3">Notification Details</h3>
          <div className="space-y-2">
            <div className="flex justify-between py-2 border-b border-gray-100">
              <span className="text-sm text-gray-600">Source</span>
              <span className="text-sm font-medium text-gray-900 capitalize">{notification.source}</span>
            </div>
            <div className="flex justify-between py-2 border-b border-gray-100">
              <span className="text-sm text-gray-600">Type</span>
              <span className="text-sm font-medium text-gray-900">{notification.notificationType}</span>
            </div>
            {notification.stepStatus && (
              <div className="flex justify-between py-2 border-b border-gray-100">
                <span className="text-sm text-gray-600">Step Status</span>
                <span className="text-sm font-medium text-gray-900 capitalize">{notification.stepStatus}</span>
              </div>
            )}
            {notification.validationStatus && (
              <div className="flex justify-between py-2 border-b border-gray-100">
                <span className="text-sm text-gray-600">Validation Status</span>
                <span className="text-sm font-medium text-gray-900 capitalize">{notification.validationStatus}</span>
              </div>
            )}
            {notification.violationType && (
              <div className="flex justify-between py-2 border-b border-gray-100">
                <span className="text-sm text-gray-600">Violation Type</span>
                <span className="text-sm font-medium text-gray-900">{notification.violationType.replace(/_/g, ' ')}</span>
              </div>
            )}
            <div className="flex justify-between py-2 border-b border-gray-100">
              <span className="text-sm text-gray-600">Timestamp</span>
              <span className="text-sm font-medium text-gray-900">
                {formatDistanceToNow(new Date(notification.timestamp), { addSuffix: true })}
              </span>
            </div>
          </div>
        </div>

        {/* Additional Details */}
        {notification.details && (() => {
          // Parse details if it's a JSON string
          let detailsObj = notification.details;
          if (typeof notification.details === 'string') {
            try {
              detailsObj = JSON.parse(notification.details);
            } catch (e) {
              detailsObj = notification.details;
            }
          }
          return Object.keys(detailsObj).length > 0 && (
            <div>
              <h3 className="text-sm font-semibold text-gray-900 mb-3">Additional Details</h3>
              <div className="bg-gray-50 border border-gray-200 rounded-lg p-4">
                <pre className="text-xs text-gray-700 overflow-x-auto whitespace-pre-wrap font-mono">
                  {JSON.stringify(detailsObj, null, 2)}
                </pre>
              </div>
            </div>
          );
        })()}

        {/* IDs */}
        <div>
          <div className="flex items-center gap-2 mb-4">
            <Hash className="w-4 h-4 text-gray-700" />
            <h3 className="text-sm font-semibold text-gray-900">Identifiers</h3>
          </div>
          
          <div className="space-y-3">
            <div>
              <div className="text-xs text-gray-600 mb-1">Notification ID:</div>
              <div className="bg-gray-50 border border-gray-200 rounded px-3 py-2 text-sm font-mono text-gray-900">
                {notification.notificationId}
              </div>
            </div>
            <div>
              <div className="text-xs text-gray-600 mb-1">Step ID:</div>
              <div className="bg-gray-50 border border-gray-200 rounded px-3 py-2 text-sm font-mono text-gray-900">
                {notification.stepId}
              </div>
            </div>
            {notification.idempotencyKey && (
              <div>
                <div className="text-xs text-gray-600 mb-1">Idempotency Key:</div>
                <div className="bg-gray-50 border border-gray-200 rounded px-3 py-2 text-sm font-mono text-gray-900">
                  {notification.idempotencyKey}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

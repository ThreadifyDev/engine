import { formatDistanceToNow } from 'date-fns';
import {
  XCircle,
  AlertTriangle,
  Info,
  Loader2,
  ChevronRight,
} from 'lucide-react';
import type { ThreadNotification, NotificationSummary } from '~/lib/graphql';

export function ValidationResultsView({ 
  notifications,
  notificationsLoading,
  notificationSummary,
  severityFilter,
  onSeverityFilterChange,
  onNotificationClick
}: { 
  notifications: ThreadNotification[];
  notificationsLoading: boolean;
  notificationSummary?: NotificationSummary;
  severityFilter: Set<string>;
  onSeverityFilterChange: (filter: Set<string>) => void;
  onNotificationClick: (notification: ThreadNotification) => void;
}) {

  if (notificationsLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="w-8 h-8 text-blue-600 animate-spin" />
      </div>
    );
  }

  // Filter notifications by severity
  const filteredNotifications = notifications.filter(n => 
    severityFilter.has(n.severity || 'unclassified')
  );

  const toggleSeverity = (severity: string) => {
    const newFilter = new Set(severityFilter);
    if (newFilter.has(severity)) {
      newFilter.delete(severity);
    } else {
      newFilter.add(severity);
    }
    onSeverityFilterChange(newFilter);
  };

  if (notifications.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12">
        <Info className="w-16 h-16 text-gray-400 mb-4" />
        <h3 className="text-lg font-semibold text-gray-900 mb-2">No Notifications</h3>
        <p className="text-sm text-gray-600">No validation notifications were returned for this run.</p>
      </div>
    );
  }

  return (
      <div className="space-y-4">
        {/* Filter Checkboxes */}
        <div className="flex flex-wrap gap-4">
          {['critical', 'major', 'warning', 'minor', 'info', 'unclassified'].map(severity => (
            <label key={severity} className="flex items-center gap-2 cursor-pointer group">
              <input type="checkbox" checked={severityFilter.has(severity)}
                onChange={() => toggleSeverity(severity)} className="w-4 h-4 rounded" />
              <span className="text-sm text-gray-700 capitalize">
                {severity} ({notifications.filter(n => (n.severity || 'unclassified') === severity).length})
              </span>
            </label>
          ))}
        </div>

        {/* Notification Cards */}
        <div className="space-y-2">
          {filteredNotifications.length === 0 && (
            <p className="text-sm text-gray-600">No notifications match the selected severities.</p>
          )}
          {filteredNotifications.map((notification) => {
            const getSeverityConfig = () => {
              switch (notification.severity) {
                case 'critical':
                  return {
                    bg: 'bg-red-50',
                    border: 'border-l-red-500',
                    icon: <XCircle className="w-4 h-4 text-red-600" />,
                    badge: 'bg-red-100 text-red-700',
                  };
                case 'warning':
                  return {
                    bg: 'bg-yellow-50',
                    border: 'border-l-yellow-500',
                    icon: <AlertTriangle className="w-4 h-4 text-yellow-600" />,
                    badge: 'bg-yellow-100 text-yellow-700',
                  };
                default:
                  return {
                    bg: 'bg-blue-50',
                    border: 'border-l-blue-500',
                    icon: <Info className="w-4 h-4 text-blue-600" />,
                    badge: 'bg-blue-100 text-blue-700',
                  };
              }
            };
            const config = getSeverityConfig();

            return (
              <button
                key={notification.notificationId}
                onClick={() => onNotificationClick(notification)}
                className={`w-full text-left ${config.bg} border-l-4 ${config.border} rounded-r-lg p-3 hover:shadow-md transition-all cursor-pointer group`}
              >
                <div className="flex items-start gap-3">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="font-medium text-gray-900 text-sm truncate">
                        {notification.stepName}
                      </span>
                      <div className="flex-shrink-0">
                        {config.icon}
                      </div>
                      <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${config.badge}`}>
                        {notification.severity || 'unclassified'}
                      </span>
                    </div>
                    <p className="text-sm text-gray-700 line-clamp-2 mb-1">
                      {notification.message}
                    </p>
                    <div className="flex items-center gap-2 text-xs text-gray-500">
                      <span className="capitalize">{notification.source}</span>
                      <span>•</span>
                      <span>{formatDistanceToNow(new Date(notification.timestamp), { addSuffix: true })}</span>
                    </div>
                  </div>
                  <ChevronRight className="w-4 h-4 text-gray-400 group-hover:text-gray-600 flex-shrink-0" />
                </div>
              </button>
            );
          })}
        </div>
      </div>
  );
}

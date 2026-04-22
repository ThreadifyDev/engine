import { AlertCircle, Info, CheckCircle, AlertTriangle } from 'lucide-react';
import React from 'react';

export type AlertType = 'error' | 'info' | 'success' | 'warning';

interface AlertAction {
  label: string;
  onClick: () => void;
  variant?: 'primary' | 'secondary';
}

interface AlertProps {
  type: AlertType;
  message: React.ReactNode;
  details?: Array<{ field: string; message: string }>;
  className?: string;
  action?: AlertAction;
}

const alertStyles: Record<AlertType, { container: string; icon: JSX.Element }> = {
  error: {
    container: 'bg-red-50 border border-red-500 text-red-900',
    icon: <AlertCircle className="h-5 w-5 text-red-800" />,
  },
  info: {
    container: 'bg-blue-50 border border-blue-500 text-blue-900',
    icon: <Info className="h-5 w-5 text-blue-800" />,
  },
  success: {
    container: 'bg-green-50 border border-green-500 text-green-900',
    icon: <CheckCircle className="h-5 w-5 text-green-800" />,
  },
  warning: {
    container: 'bg-yellow-50 border border-yellow-500 text-yellow-900',
    icon: <AlertTriangle className="h-5 w-5 text-yellow-800" />,
  },
};

export default function Alert({ type, message, details, className = '', action }: AlertProps) {
  const styles = alertStyles[type];

  return (
    <div className={`flex flex-col gap-3 px-4 py-3 rounded-lg ${styles.container} ${className}`}>
      <div className="flex items-start gap-3">
        <div className="flex-shrink-0 mt-0.5">{styles.icon}</div>
        <div className="flex-1 text-sm font-medium">{message}</div>
      </div>
      {action && (
        <div className="ml-8">
          <button
            onClick={action.onClick}
            className={`text-sm font-medium px-4 py-2 rounded-lg transition-colors ${
              action.variant === 'secondary'
                ? 'bg-white/50 hover:bg-white/70 text-gray-900'
                : 'bg-gray-900 hover:bg-gray-800 text-white'
            }`}
          >
            {action.label}
          </button>
        </div>
      )}
      {details && details.length > 0 && (
        <ul className="ml-8 space-y-1 text-sm">
          {details.map((detail, index) => (
            <li key={index} className="flex items-center gap-2">
              <span className="font-medium capitalize">{detail.field}:</span>
              <span>{detail.message}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// Helper function to detect credit-related errors
export function isCreditError(message: string): boolean {
  const lower = message.toLowerCase();
  return (
    lower.includes('insufficient credit') ||
    lower.includes('no credit') ||
    lower.includes('low credit') ||
    lower.includes('please top up') ||
    lower.includes('please purchase credit') ||
    lower.includes('no billing account') ||
    lower.includes('set up billing') ||
    lower.includes('payment required')
  );
}

import { AlertCircle, Info, CheckCircle, AlertTriangle } from 'lucide-react';

export type AlertType = 'error' | 'info' | 'success' | 'warning';

interface AlertProps {
  type: AlertType;
  message: string;
  details?: Array<{ field: string; message: string }>;
  className?: string;
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

export default function Alert({ type, message, details, className = '' }: AlertProps) {
  const styles = alertStyles[type];

  return (
    <div className={`flex flex-col gap-3 px-4 py-3 rounded-lg ${styles.container} ${className}`}>
      <div className="flex items-start gap-3">
        <div className="flex-shrink-0 mt-0.5">{styles.icon}</div>
        <div className="flex-1 text-sm capitalize font-medium">{message}</div>
      </div>
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

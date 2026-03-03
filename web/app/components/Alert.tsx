import { AlertCircle, Info, CheckCircle, AlertTriangle } from 'lucide-react';

export type AlertType = 'error' | 'info' | 'success' | 'warning';

interface AlertProps {
  type: AlertType;
  message: string;
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

export default function Alert({ type, message, className = '' }: AlertProps) {
  const styles = alertStyles[type];

  return (
    <div className={`flex items-start gap-3 px-4 py-3 rounded-lg capitalize ${styles.container} ${className}`}>
      <div className="flex-shrink-0 mt-0.5">{styles.icon}</div>
      <div className="flex-1 text-sm">{message}</div>
    </div>
  );
}

import { X } from 'lucide-react';
import { ReactNode } from 'react';

interface RightSidebarProps {
  isOpen: boolean;
  onClose: () => void;
  title: string | ReactNode;
  children: ReactNode;
  width?: 'sm' | 'md' | 'lg';
}

export default function RightSidebar({ 
  isOpen, 
  onClose, 
  title, 
  children,
  width = 'md'
}: RightSidebarProps) {
  const widthClasses = {
    sm: 'w-80',
    md: 'w-96',
    lg: 'w-[32rem]'
  };

  if (!isOpen) return null;

  return (
    <>
      {/* Backdrop */}
      <div 
        className="fixed inset-0 bg-black/10 z-40"
        onClick={onClose}
      />
      
      {/* Sidebar */}
      <div 
        className={`fixed right-0 top-0 h-full ${widthClasses[width]} bg-white border-l border-gray-200 shadow-xl overflow-y-auto z-50 animate-slide-in-right`}
      >
        {/* Header */}
        <div className="sticky top-0 bg-white border-b border-gray-200 p-4 flex items-center justify-between z-10">
          <h3 className="font-semibold text-lg text-gray-900">{title}</h3>
          <button
            onClick={onClose}
            className="hover:bg-gray-100 p-1.5 rounded-md transition-colors"
          >
            <X className="w-5 h-5 text-gray-500" />
          </button>
        </div>

        {/* Content */}
        <div className="p-6">
          {children}
        </div>
      </div>
    </>
  );
}

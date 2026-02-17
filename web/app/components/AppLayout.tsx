import { ReactNode, useState } from 'react';
import SideNav from './SideNav';

interface AppLayoutProps {
  children: ReactNode;
  showRightSidebar?: boolean;
  rightSidebarContent?: ReactNode;
  rightSidebarWidth?: string;
}

export default function AppLayout({ 
  children, 
  showRightSidebar = false,
  rightSidebarContent,
  rightSidebarWidth = '400px'
}: AppLayoutProps) {
  const [isNavCollapsed, setIsNavCollapsed] = useState(true);

  const navWidth = isNavCollapsed ? 64 : 256; // w-16 = 64px, w-64 = 256px

  return (
    <div className="min-h-screen bg-white flex">
      {/* Left Navigation */}
      <SideNav 
        isCollapsed={isNavCollapsed} 
        onToggle={() => setIsNavCollapsed(!isNavCollapsed)} 
      />
      
      {/* Main Content Area */}
      <main 
        className="flex-1 transition-all duration-300 overflow-auto"
        style={{
          marginLeft: `${navWidth}px`,
          marginRight: showRightSidebar ? rightSidebarWidth : '0',
        }}
      >
        {children}
      </main>

      {/* Right Sidebar (Optional) */}
      {showRightSidebar && (
        <aside 
          className="fixed right-0 top-0 h-screen bg-white border-l border-gray-200 overflow-y-auto transition-all duration-300 z-10"
          style={{ width: rightSidebarWidth }}
        >
          {rightSidebarContent}
        </aside>
      )}
    </div>
  );
}

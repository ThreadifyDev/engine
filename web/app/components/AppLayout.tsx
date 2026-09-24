import { type CSSProperties, ReactNode, useEffect, useRef, useState } from 'react';
import SideNav from './SideNav';
import TopHeader from './TopHeader';
import AgentToggleButton from './agent/AgentToggleButton';
import { useAgent } from './agent/agent-context';

interface AppLayoutProps {
  children: ReactNode;
  hideDesktopHeader?: boolean;
  showRightSidebar?: boolean;
  rightSidebarContent?: ReactNode;
  rightSidebarWidth?: string;
}

export default function AppLayout({
  children,
  hideDesktopHeader = true,
  showRightSidebar = false,
  rightSidebarContent,
  rightSidebarWidth = '400px'
}: AppLayoutProps) {
  const [isNavCollapsed, setIsNavCollapsed] = useState(true);
  const [isMobileOpen, setIsMobileOpen] = useState(false);
  const { isOpen: agentOpen, isCompact, closeAgent } = useAgent();
  const content = useRef<HTMLDivElement>(null);
  useEffect(() => {
    content.current?.toggleAttribute('inert', agentOpen && isCompact);
  }, [agentOpen, isCompact]);

  // navWidth only applies to desktop (lg and up)
  const navWidth = isNavCollapsed ? 64 : 256; 

  return (
    <div className="min-h-screen min-w-0 w-full bg-white flex">
      {/* Mobile Nav Overlay */}
      {isMobileOpen && (
        <div 
          className="fixed inset-0 bg-black/50 z-40 lg:hidden"
          onClick={() => setIsMobileOpen(false)}
        />
      )}

      {/* Left Navigation */}
      <SideNav 
        isCollapsed={isNavCollapsed} 
        onToggle={() => setIsNavCollapsed(!isNavCollapsed)} 
        isMobileOpen={isMobileOpen}
        onCloseMobile={() => setIsMobileOpen(false)}
      />
      
      {/* Main Content Area */}
      <main 
        className="min-w-0 w-full flex-1"
        style={{ "--nav-width": `${navWidth}px` } as CSSProperties}
      >
        {/* Mobile Header */}
        <div className="sticky top-0 z-[80] flex h-14 items-center justify-between border-b border-gray-200 bg-white px-4 lg:hidden">
          <div className="flex items-center">
            <button
              aria-label="Open navigation"
              onClick={() => { closeAgent(); setIsMobileOpen(true); }}
              className="p-2 hover:bg-gray-100 rounded text-black transition-colors mr-3"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="3" y1="12" x2="21" y2="12"></line><line x1="3" y1="6" x2="21" y2="6"></line><line x1="3" y1="18" x2="21" y2="18"></line></svg>
            </button>
            <span className="font-semibold">Threadify</span>
          </div>
          <AgentToggleButton />
        </div>

        <div className="min-w-0 w-full transition-[padding] duration-300 lg:pl-[var(--nav-width)]">
          {!hideDesktopHeader && <div className="sticky top-0 z-30 hidden lg:block">
            <TopHeader />
          </div>}
          <div ref={content} className={`min-w-0 max-w-full break-words transition-[padding] duration-200 motion-reduce:transition-none ${agentOpen ? 'lg:pr-[360px] xl:pr-[420px]' : ''}`}>{children}</div>
        </div>
      </main>

      {/* Right Sidebar (Optional) */}
      {showRightSidebar && (
        <aside 
          className="fixed right-0 top-0 h-screen bg-white border-l border-gray-200 overflow-y-auto transition-all duration-300 z-10"
          style={{ width: rightSidebarWidth, maxWidth: '100%' }}
        >
          {rightSidebarContent}
        </aside>
      )}
    </div>
  );
}

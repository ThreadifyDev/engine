import { useState, useRef, useEffect } from 'react';
import { useNavigate } from '@remix-run/react';
import { Wallet, ChevronRight } from 'lucide-react';
import { useCurrentPlan } from '~/hooks/useBilling';

export default function TopHeader() {
  const navigate = useNavigate();
  const { data: billingData } = useCurrentPlan();
  const [isOpen, setIsOpen] = useState(false);
  const [position, setPosition] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  
  const walletRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef({
    isDragging: false,
    startX: 0,
    startY: 0,
    startPosX: 0,
    startPosY: 0,
    hasDragged: false
  });

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (walletRef.current && !walletRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isOpen]);

  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!dragRef.current.isDragging) return;
      
      const dx = e.clientX - dragRef.current.startX;
      const dy = e.clientY - dragRef.current.startY;
      
      if (!dragRef.current.hasDragged && (Math.abs(dx) > 3 || Math.abs(dy) > 3)) {
        dragRef.current.hasDragged = true;
        setIsDragging(true);
      }
      
      if (dragRef.current.hasDragged) {
        setPosition({
          x: dragRef.current.startPosX + dx,
          y: dragRef.current.startPosY + dy
        });
      }
    };

    const handleMouseUp = () => {
      if (dragRef.current.isDragging) {
        dragRef.current.isDragging = false;
        setIsDragging(false);
      }
    };

    document.addEventListener('mousemove', handleMouseMove);
    document.addEventListener('mouseup', handleMouseUp);
    
    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, []);

  const handleMouseDown = (e: React.MouseEvent) => {
    if (e.button !== 0) return; // Only left click
    
    // Don't drag if clicking the balance button text area
    if ((e.target as HTMLElement).closest('.balance-btn')) return;

    dragRef.current = {
      isDragging: true,
      startX: e.clientX,
      startY: e.clientY,
      startPosX: position.x,
      startPosY: position.y,
      hasDragged: false
    };
  };

  const handleWalletClick = () => {
    if (!dragRef.current.hasDragged) {
      setIsOpen(!isOpen);
    }
  };

  const formatBalance = (millicents: number) => {
    const dollars = millicents / 100000;
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(dollars);
  };

  const balance = billingData?.credit_account?.balance_millicents ?? 0;
  const minBalance = billingData?.credit_account?.min_balance_millicents ?? 0;
  const isLow = balance > 0 && balance < minBalance;

  // Don't render the floating wallet if there's no billing account active
  if (!billingData?.credit_account) return null;

  return (
    <div 
      ref={walletRef}
      onMouseDown={handleMouseDown}
      className={`fixed bottom-6 right-6 z-50 flex items-center shadow-lg rounded-full bg-white border border-gray-200 transition-all duration-300 h-12 select-none ${
        !isOpen ? 'opacity-40 hover:opacity-100' : 'opacity-100 shadow-xl'
      } ${isDragging ? 'cursor-grabbing transition-none shadow-xl opacity-100' : 'cursor-grab'}`}
      style={{
        transform: `translate(${position.x}px, ${position.y}px)`,
        // If dragging, we disable transition so it moves instantly with mouse
      }}
    >
      <button
        onClick={handleWalletClick}
        className={`flex items-center justify-center w-12 h-12 rounded-full transition-colors ${
          isLow ? 'text-yellow-600 hover:bg-yellow-50' : 'text-gray-700 hover:bg-gray-50'
        } ${isDragging ? 'cursor-grabbing pointer-events-none' : 'cursor-pointer'}`}
        title="Wallet Balance"
      >
        <Wallet className="w-5 h-5" />
      </button>
      
      <div 
        className={`overflow-hidden transition-all duration-300 flex items-center ${
          isOpen ? 'max-w-xs pr-2 opacity-100' : 'max-w-0 pr-0 opacity-0'
        }`}
      >
        <div className="h-6 w-px bg-gray-200 mx-1"></div>
        <button
          onClick={(e) => {
            e.stopPropagation();
            navigate('/u/settings?tab=billing');
          }}
          className="balance-btn flex items-center gap-2 px-3 py-1.5 hover:bg-gray-50 rounded-full transition-colors whitespace-nowrap cursor-pointer"
        >
          <span className={`text-sm font-semibold ${isLow ? 'text-yellow-600' : 'text-gray-900'}`}>
            {formatBalance(balance)}
          </span>
          <ChevronRight className="w-4 h-4 text-gray-400" />
        </button>
      </div>
    </div>
  );
}

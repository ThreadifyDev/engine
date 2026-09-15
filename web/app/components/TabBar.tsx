import type { KeyboardEvent, ReactNode } from 'react';

type TabItem<T extends string> = { value: T; label: ReactNode };

/** Content navigation uses an underline, while ordinary actions keep their button styling. */
export function TabBar<T extends string>({ label, value, items, onChange, panelId, className = '' }: {
  label: string;
  value: T;
  items: TabItem<T>[];
  onChange: (value: T) => void;
  panelId: string;
  className?: string;
}) {
  function navigate(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next = index;
    if (event.key === 'ArrowRight') next = (index + 1) % items.length;
    else if (event.key === 'ArrowLeft') next = (index - 1 + items.length) % items.length;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = items.length - 1;
    else return;
    event.preventDefault();
    onChange(items[next].value);
    const tabs = event.currentTarget.parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]');
    tabs?.[next]?.focus();
  }
  return <div role="tablist" aria-label={label} className={`flex min-w-0 gap-5 overflow-x-auto border-b border-gray-200 ${className}`}>
    {items.map((item, index) => <button
      key={item.value}
      type="button"
      role="tab"
      aria-selected={value === item.value}
      aria-controls={panelId}
      tabIndex={value === item.value ? 0 : -1}
      onClick={() => onChange(item.value)}
      onKeyDown={event => navigate(event, index)}
      className={`inline-flex shrink-0 items-center gap-2 whitespace-nowrap border-b-2 bg-transparent px-1 py-2.5 text-[13px] font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-gray-500 ${value === item.value ? 'border-gray-900 text-gray-950' : 'border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-900'}`}
    >{item.label}</button>)}
  </div>;
}

import type { KeyboardEvent, ReactNode } from 'react';

type TabItem<T extends string> = { value: T; label: ReactNode };

/** Keyboard accessible content navigation shared by every tabbed view. */
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
  return <div role="tablist" aria-label={label} className={`flex w-full min-w-0 items-center gap-1 overflow-x-auto rounded-xl border border-stone-200 bg-white p-1.5 shadow-sm ${className}`}>
    {items.map((item, index) => <button
      key={item.value}
      type="button"
      role="tab"
      aria-selected={value === item.value}
      aria-controls={panelId}
      tabIndex={value === item.value ? 0 : -1}
      onClick={() => onChange(item.value)}
      onKeyDown={event => navigate(event, index)}
      className={`inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-lg px-4 py-2.5 text-sm font-medium leading-5 transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-600 focus-visible:ring-offset-2 ${value === item.value
        ? 'bg-stone-900 text-white shadow-sm'
        : 'text-stone-500 hover:bg-stone-50 hover:text-stone-900'}`}
    ><span aria-hidden="true" className={`h-1.5 w-1.5 rounded-full ${value === item.value ? 'bg-emerald-400' : 'bg-transparent'}`} />{item.label}</button>)}
  </div>;
}

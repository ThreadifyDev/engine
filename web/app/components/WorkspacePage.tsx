import type { ReactNode } from 'react';
import AppLayout from './AppLayout';

type Props = {
  eyebrow: string;
  title: string;
  description: string;
  actions?: ReactNode;
  children: ReactNode;
  wide?: boolean;
};

export default function WorkspacePage({ eyebrow, title, description, actions, children, wide = false }: Props) {
  return (
    <AppLayout>
      <div className="min-h-screen bg-[#f8f8f6] px-4 py-7 sm:px-7 sm:py-10 lg:px-10">
        <div className={wide ? 'mx-auto max-w-7xl' : 'mx-auto max-w-6xl'}>
          <header className="mb-9 flex flex-wrap items-start justify-between gap-5">
            <div className="min-w-0">
              <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-700">{eyebrow}</p>
              <h1 className="break-words text-3xl font-semibold tracking-tight text-stone-950 sm:text-4xl">{title}</h1>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-stone-500">{description}</p>
            </div>
            {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
          </header>
          {children}
        </div>
      </div>
    </AppLayout>
  );
}

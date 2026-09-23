import { lazy, type ComponentType, type LazyExoticComponent } from 'react';

export type Page = {
  path: string;
  agentSupported?: boolean;
  title: string | ((params: Readonly<Record<string, string | undefined>>) => string);
  component: LazyExoticComponent<ComponentType>;
};

// Explicit browser routes keep deep links stable without a server-side router.
export const pages: Page[] = [
  { path: '/login', title: 'Sign in', component: lazy(() => import('./routes/login')) },
  { path: '/cli-login', title: 'CLI sign in', component: lazy(() => import('./routes/cli-login')) },
  { path: '/u/dashboard', title: 'Dashboard', component: lazy(() => import('./routes/u.dashboard')) },
  { path: '/u/threads', agentSupported: true, title: 'Threads', component: lazy(() => import('./routes/u.threads')) },
  { path: '/u/threads/:id', agentSupported: true, title: 'Thread Details', component: lazy(() => import('./routes/u.threads_.$id')) },
  { path: '/u/contracts', agentSupported: true, title: 'Contracts', component: lazy(() => import('./routes/u.contracts')) },
  { path: '/u/contracts/:id', agentSupported: true, title: 'Contract', component: lazy(() => import('./routes/u.contracts_.$id')) },
  { path: '/u/contracts/:id/versions/:version', agentSupported: true, title: 'Contract Version', component: lazy(() => import('./routes/u.contracts_.$id_.versions.$version')) },
  { path: '/u/profile-views/:type', agentSupported: true, title: params => `${params.type} · Profile configuration`, component: lazy(() => import('./routes/u.profile-views_.$type')) },
  { path: '/u/profiles', agentSupported: true, title: 'Entity Profiles', component: lazy(() => import('./routes/u.profiles')) },
  { path: '/u/profiles/:type', agentSupported: true, title: params => `${params.type} · Entity Profiles`, component: lazy(() => import('./routes/u.profiles_.$type')) },
  { path: '/u/profiles/:type/:refKey', agentSupported: true, title: params => `Profile ${params.refKey}`, component: lazy(() => import('./routes/u.profiles_.$type_.$refKey')) },
  { path: '/u/team', title: 'Team', component: lazy(() => import('./routes/u.team')) },
  { path: '/u/settings', title: 'Settings', component: lazy(() => import('./routes/u.settings')) },
  { path: '/u/developer', title: 'Developer', component: lazy(() => import('./routes/u.developer')) },
  { path: '/u/api-keys', title: 'API Keys', component: lazy(() => import('./routes/u.api-keys')) },
  { path: '/u/service-accounts', title: 'Service Accounts', component: lazy(() => import('./routes/u.service-accounts')) },
  { path: '/u/onboarding', title: 'Onboarding', component: lazy(() => import('./routes/u.onboarding')) },
  { path: '/u/getting-started', title: 'Getting Started', component: lazy(() => import('./routes/u.getting-started')) },
];

export const redirects: Record<string, string> = {
  '/': '/u/dashboard',
  '/u': '/u/dashboard',
  '/u/assistant': '/u/dashboard',
  '/signup': '/login',
  '/auth/forgot-password': '/login',
  '/auth/reset-password': '/login',
  '/auth/verify-otp': '/login',
};

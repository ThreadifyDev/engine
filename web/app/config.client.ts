// Client-side configuration
// This reads from window.__ENV__ which is injected by the server

export const getConfig = () => {
  if (typeof window === 'undefined') {
    throw new Error('getConfig() can only be called on the client');
  }
  if (!(window as any).__ENV__?.API_URL) {
    throw new Error('Runtime configuration not found. Ensure window.__ENV__.API_URL is set by the server.');
  }
  if (!(window as any).__ENV__?.ENGINE_URL) {
    throw new Error('Engine URL is missing from runtime configuration.');
  }
  return {
    apiUrl: (window as any).__ENV__.API_URL,
    engineUrl: (window as any).__ENV__.ENGINE_URL,
  };
};

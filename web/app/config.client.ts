// Client-side configuration
// This reads from window.__ENV__ which is injected by the server

export const getConfig = () => {
  if (typeof window === 'undefined') {
    throw new Error('getConfig() can only be called on the client');
  }
  
  return {
    apiUrl: (window as any).__ENV__?.API_URL || 'http://localhost:3001',
  };
};

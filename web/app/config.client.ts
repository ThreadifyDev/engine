// Embedded dashboards use the origin serving the binary. Explicit overrides
// remain available for a separately hosted dashboard.
export const getConfig = () => {
  if (typeof window === 'undefined') throw new Error('getConfig() can only be called on the client');
  const overrides = (window as any).__ENV__;
  const engineUrl = overrides?.ENGINE_URL || window.location.origin;
  return { engineUrl, apiUrl: overrides?.API_URL || engineUrl };
};

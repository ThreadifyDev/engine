// Server-side configuration
// This file is only imported on the server, never bundled to the client

export const getConfig = () => {
  return {
    apiUrl: process.env.API_URL || 'http://localhost:3001',
    engineUrl: process.env.ENGINE_URL || 'http://localhost:8081',
    harnestUrl: process.env.HARNEST_URL || 'http://127.0.0.1:8090',
  };
};

export type Config = ReturnType<typeof getConfig>;

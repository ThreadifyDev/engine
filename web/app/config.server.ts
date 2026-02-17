// Server-side configuration
// This file is only imported on the server, never bundled to the client

export const getConfig = () => {
  return {
    apiUrl: process.env.API_URL || 'http://localhost:3001',
  };
};

export type Config = ReturnType<typeof getConfig>;

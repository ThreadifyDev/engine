# Runtime Configuration Setup

To enable runtime configuration (change API URL without rebuilding), follow these steps:

## 1. Update `app/root.tsx`

Add this to your root loader to inject environment variables:

```typescript
import { getConfig } from './config.server';

export const loader = async () => {
  const config = getConfig();
  
  return json({
    ENV: {
      API_URL: config.apiUrl,
    },
  });
};

export default function App() {
  const data = useLoaderData<typeof loader>();
  
  return (
    <html lang="en">
      <head>
        {/* ... existing head content ... */}
      </head>
      <body>
        {/* Inject ENV into window */}
        <script
          dangerouslySetInnerHTML={{
            __html: `window.__ENV__ = ${JSON.stringify(data.ENV)}`,
          }}
        />
        <Outlet />
        {/* ... existing body content ... */}
      </body>
    </html>
  );
}
```

## 2. Update `app/lib/api.ts`

Replace the hardcoded API URL:

```typescript
// OLD:
const API_BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:3001/api';

// NEW:
import { getConfig } from '../config.client';

const getApiBaseUrl = () => {
  if (typeof window !== 'undefined') {
    return getConfig().apiUrl + '/api';
  }
  return 'http://localhost:3001/api'; // SSR fallback
};

const API_BASE_URL = getApiBaseUrl();
```

## 3. Update `app/lib/graphql.ts`

Replace the hardcoded API URL (around line 185-187):

```typescript
// OLD:
const apiUrl = typeof window !== 'undefined' 
  ? (import.meta.env.VITE_API_URL || 'http://localhost:3001')
  : 'http://localhost:3001';

// NEW:
import { getConfig } from '../config.client';

const apiUrl = typeof window !== 'undefined' 
  ? getConfig().apiUrl
  : 'http://localhost:3001';
```

## 4. Build and Deploy

Now you can:

```bash
# Build ONCE
docker build -f threadify-go/Dockerfile.web_ui -t intelijence/th_web_ui:latest .

# Deploy to different environments with different API URLs
# Development:
docker run -e API_URL=http://localhost:3001 -p 3000:3000 intelijence/th_web_ui:latest

# Production:
docker run -e API_URL=https://web.threadify.dev -p 3000:3000 intelijence/th_web_ui:latest
```

Or in docker-compose, just change the `.env` file:
```bash
API_URL=https://web.threadify.dev
```

No rebuild needed! 🎉

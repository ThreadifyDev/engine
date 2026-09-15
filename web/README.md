# Threadify dashboard

Requires **Node.js 24 LTS**. The dashboard connects to a separately running
Threadify Engine.

## Develop

From this directory:

```sh
nvm install
nvm use
npm ci
npm run dev
```

Configure the Engine connection as described in [ENGINE_CONNECTION.md](ENGINE_CONNECTION.md).
See [browser authentication](../threadify-go/docs/BROWSER_AUTH.md) for Registry
sign-in and deployment origins.

## Build and test

```sh
npm run test:engine-routing
npm run test:auth
npm start
```

`test:auth` builds the production application before checking browser authentication.
The web CI workflow uses Node 24 and runs both test commands after `npm ci`.

## Docker

From the repository root:

```sh
docker build -f threadify-go/Dockerfile.web_ui -t threadify-web web
```

Both build and production stages use `node:24-alpine`. Runtime configuration is
explained in [RUNTIME_CONFIG_SETUP.md](RUNTIME_CONFIG_SETUP.md).

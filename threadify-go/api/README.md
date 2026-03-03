# Threadify Web API

Go/Gin backend API for Threadify Web application.

## Setup

1. Install dependencies:
```bash
go mod tidy
```

2. Copy `.env.example` to `.env` and configure:
```bash
cp .env.example .env
```

3. Run the server:
```bash
go run cmd/server/main.go
```

Server will start on `http://localhost:3001`

## Project Structure

```
api/
├── cmd/
│   └── server/          # Main application entry point
├── config/              # Configuration management
├── internal/
│   ├── handlers/        # HTTP request handlers
│   ├── middleware/      # HTTP middleware
│   ├── models/          # Data models
│   ├── repository/      # Database access layer
│   ├── services/        # Business logic
│   └── utils/           # Helper functions
└── migrations/          # Database migrations
```

## Development

- **Health check**: `GET /health`
- **API base**: `/api`

## Auth Integration

Authentication and authorization run through the configured auth provider.

Set these values under `web_api.auth` in your config (or `web_api.auth0` for backward compatibility):

- `enabled`
- `domain`
- `client_id`
- `client_secret`
- `database_connection`
- `audience` (optional)
- `management_client_id`
- `management_client_secret`
- `management_audience` (optional; defaults to `https://<domain>/api/v2/`)
- `request_timeout_seconds`

Current behavior:

- `POST /api/auth/signup`: creates local tenant/user and auth-provider credentials
- `POST /api/auth/login`: authenticates with auth provider and returns access token
- `POST /api/auth/forgot-password`: triggers provider password reset flow

## Authentication Endpoints

- `POST /api/auth/signup` - Create account
- `POST /api/auth/login` - Authenticate and receive access token
- `POST /api/auth/forgot-password` - Trigger provider password reset

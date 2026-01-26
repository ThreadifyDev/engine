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

## Phase 1: Authentication (In Progress)

- `POST /api/auth/signup` - Create account
- `POST /api/auth/login` - Authenticate user
- `POST /api/auth/verify-otp` - Verify email OTP

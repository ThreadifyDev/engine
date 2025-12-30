# Threadify Docker Deployment

This guide explains how to run Threadify using Docker and Docker Compose.

## Prerequisites

- Docker 20.10+
- Docker Compose 2.0+

## Quick Start

### 1. Clone the repository

```bash
git clone <repository-url>
cd ThreadifyEngine
```

### 2. Create environment file

```bash
cp .env.example .env
```

Edit `.env` and update the values as needed (especially passwords and JWT secret for production).

### 3. Start all services

```bash
docker-compose up -d
```

This will start:
- **PostgreSQL (Citus)** - Main database on port 5434
- **Valkey (Redis)** - Cache and queue on port 6379
- **Threadify Engine** - API server on port 8081
- **Threadify Archiver** - Background data archival service

### 4. Check service health

```bash
docker-compose ps
```

All services should show as "healthy" after a few seconds.

### 5. View logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f threadify-engine
```

## Services

### Threadify Engine
- **Port**: 8081 (HTTP/WebSocket)
- **Port**: 6060 (pprof profiling)
- **Health**: http://localhost:8081/health

### PostgreSQL (Citus)
- **Port**: 5434
- **Database**: threadify
- **User**: td_engine (configurable via .env)

### Valkey (Redis)
- **Port**: 6379
- **Password**: threadify_secure_password (configurable via .env)

## Management Commands

### Stop all services
```bash
docker-compose down
```

### Stop and remove volumes (⚠️ deletes all data)
```bash
docker-compose down -v
```

### Rebuild Threadify Engine
```bash
docker-compose build threadify-engine
docker-compose up -d threadify-engine
```

### View real-time logs
```bash
docker-compose logs -f threadify-engine
```

### Execute commands in container
```bash
docker-compose exec threadify-engine sh
```

### Restart a service
```bash
docker-compose restart threadify-engine
```

## Development

### Local development with Docker databases

If you want to run the Go server locally but use Docker for databases:

```bash
# Start only databases
docker-compose up -d postgres valkey

# Run Go server locally
cd threadify-go
go run cmd/server/main.go
```

### Rebuild after code changes

```bash
docker-compose build threadify-engine
docker-compose up -d threadify-engine
```

## Configuration

### Environment Variables

Key environment variables (set in `.env`):

- `PS_STORAGE_USER` - PostgreSQL username
- `PS_STORAGE_PASSWORD` - PostgreSQL password
- `FAST_STORAGE_PASSWORD` - Valkey/Redis password
- `JWT_SECRET` - JWT signing secret (⚠️ change in production!)
- `LOG_LEVEL` - Logging level (debug, info, warn, error)

### Config Files

- `threadify-go/config/config.yaml` - Local development config
- `threadify-go/config/config.docker.yaml` - Docker deployment config

## Troubleshooting

### Service won't start

Check logs:
```bash
docker-compose logs threadify-engine
```

### Database connection issues

Ensure PostgreSQL is healthy:
```bash
docker-compose ps postgres
```

Check connection:
```bash
docker-compose exec postgres psql -U td_engine -d threadify -c "SELECT 1;"
```

### Valkey connection issues

Test Valkey:
```bash
docker-compose exec valkey valkey-cli -a threadify_secure_password ping
```

### Port conflicts

If ports 5434, 6379, or 8081 are already in use, modify the port mappings in `docker-compose.yml`:

```yaml
ports:
  - "5435:5432"  # Change 5434 to 5435
```

## Production Deployment

For production:

1. **Change all default passwords** in `.env`
2. **Set a strong JWT_SECRET** (minimum 32 characters)
3. **Enable SSL/TLS** for PostgreSQL
4. **Use secrets management** instead of `.env` file
5. **Set up monitoring** and log aggregation
6. **Configure backups** for PostgreSQL volumes
7. **Use a reverse proxy** (nginx/traefik) with SSL

## Network

All services communicate on the `threadify-network` bridge network. This provides:
- Service discovery by name (e.g., `postgres`, `valkey`)
- Network isolation
- DNS resolution

## Volumes

Persistent data is stored in Docker volumes:
- `threadify-persist` - PostgreSQL data
- `threadify-queue` - Valkey data

To backup:
```bash
docker run --rm -v threadify-persist:/data -v $(pwd):/backup alpine tar czf /backup/postgres-backup.tar.gz /data
```

## Health Checks

All services have health checks configured:
- **PostgreSQL**: `pg_isready` check every 10s
- **Valkey**: `ping` check every 10s
- **Threadify Engine**: HTTP health endpoint check every 30s

## Support

For issues or questions, please check the main README.md or open an issue on GitHub.

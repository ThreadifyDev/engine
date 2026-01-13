# Configuration Structure

This directory contains all configuration files for the Threadify Engine deployment.

## Directory Structure

```
config/
├── valkey/
│   └── valkey.conf          # Valkey/Redis configuration
└── docker/
    └── config.docker.yaml   # Docker environment configuration
```

## Configuration Files

### Valkey Configuration (`config/valkey/valkey.conf`)
- **Purpose**: Valkey/Redis server configuration
- **Used by**: `threadify-queue` service
- **Mount path**: `/etc/valkey/valkey.conf`
- **Key settings**:
  - Authentication password: `threadify_secure_password`
  - Memory limit: 1024MB
  - AOF persistence enabled
  - Performance optimizations for high throughput

### Docker Configuration (`config/docker/config.docker.yaml`)
- **Purpose**: Threadify services configuration for containerized environments
- **Used by**: `threadify-engine` and `threadify-archiver` services
- **Mount paths**: 
  - Engine: `/app/config/config.yaml`
  - Archiver: `/root/config/config.yaml`
- **Key sections**:
  - PostgreSQL connection settings
  - Redis/Valkey connection settings
  - NATS message broker configuration
  - JWT authentication settings
  - Archiver service configuration

## Environment Variables

The configuration supports environment variable overrides:

- `PS_STORAGE_USER`: PostgreSQL username (default: `td_engine`)
- `PS_STORAGE_PASSWORD`: PostgreSQL password (default: `tdtdtd`)
- `FAST_STORAGE_PASSWORD`: Redis/Valkey password (default: `threadify_secure_password`)

## Docker Compose Integration

The `docker-compose.yml` file mounts these configurations as read-only volumes:

```yaml
services:
  threadify-queue:
    volumes:
      - ./config/valkey/valkey.conf:/etc/valkey/valkey.conf:ro
  
  threadify-engine:
    volumes:
      - ./config/docker:/app/config:ro
  
  threadify-archiver:
    volumes:
      - ./config/docker:/root/config:ro
```

## Configuration Updates

To modify configuration:

1. **Runtime changes**: Edit files in this directory and restart services
   ```bash
   docker compose restart [service-name]
   ```

2. **Environment-specific overrides**: Use environment variables or create docker-compose override files

3. **Security considerations**: 
   - Change default passwords before production deployment
   - Use environment variables for sensitive data
   - Keep configuration files out of version control if they contain secrets

## Notes

- Configuration changes don't require rebuilding Docker images
- All services read configuration at startup from mounted volumes
- The archiver service uses a different mount path (`/root/config`) due to its WORKDIR setting
- Consider standardizing mount paths in future updates for consistency

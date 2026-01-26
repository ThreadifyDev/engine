# Database Migrations

## Automated Migrations (golang-migrate)

Migrations run **automatically** when the server starts. The system uses `golang-migrate` to track and apply database changes.

### How It Works

1. **On Server Start**: Migrations run automatically before the server accepts requests
2. **Version Tracking**: Migration version stored in `schema_migrations` table
3. **Idempotent**: Safe to run multiple times - only applies pending migrations
4. **Rollback Support**: Can rollback migrations if needed

### Migration Files

Migrations follow the naming pattern: `{version}_{description}.{up|down}.sql`

- `000001_create_web_auth_tables.up.sql` - Create web auth tables (companies_web, users_web, etc.)
- `000001_create_web_auth_tables.down.sql` - Rollback migration

### Production Deployment

**Migrations run automatically on deployment:**

1. Container starts
2. Server connects to database
3. Migrations run (if any pending)
4. Server starts accepting requests

**Zero-downtime deployments:**
- Write backwards-compatible migrations
- Add columns as nullable first
- Drop columns in separate migration after deploy

### Manual Operations

**Check migration status:**
```bash
psql "$DATABASE_URL" -c "SELECT * FROM schema_migrations;"
```

**Rollback last migration (if needed):**
```go
// In code or create a CLI tool
utils.RollbackMigration(db, "./migrations")
```

### Creating New Migrations

1. Create two files:
   - `{next_version}_{description}.up.sql` - Apply changes
   - `{next_version}_{description}.down.sql` - Revert changes

2. Example:
```bash
# 000002_add_user_preferences.up.sql
ALTER TABLE users_web ADD COLUMN preferences JSONB;

# 000002_add_user_preferences.down.sql
ALTER TABLE users_web DROP COLUMN preferences;
```

3. Restart server or deploy - migration runs automatically

## Table Naming Convention

**Web App Tables** (separate from ThreadifyEngine):
- `companies_web` - Web app companies
- `users_web` - Web app users
- `otp_codes` - OTP verification codes
- `service_accounts` - Non-human identities
- `api_keys` - API authentication keys

**Why separate tables?**
- Avoids conflicts with ThreadifyEngine's existing `companies` and `users` tables
- Clear separation of concerns
- Independent schema evolution

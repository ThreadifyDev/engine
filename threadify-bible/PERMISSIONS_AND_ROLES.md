# Threadify Permission and Role Model

## Overview

Threadify implements a sophisticated two-layer permission system that separates API-level access control from runtime thread permissions and notification filtering.

## Architecture Layers

### Layer 1: API-Level Permissions (app_level)

Controls what APIs and resources a user/service account can access.

**Permission Pattern**: `resource.action.scope`

Examples:
- `contract.read.*` - Read all contracts
- `contract.read.product_delivery` - Read only product_delivery contract
- `query.execution.*` - Execute GraphQL queries for all contracts
- `apikey.create` - Create API keys

### Layer 2: Runtime Permissions (runtime_level)

Controls what actions can be performed within a thread and what notifications are received.

**Permission Pattern**: `category.subcategory.scope`

Examples:
- `notification.violations.*` - All violation notifications
- `notification.violations.own` - Only own step violations
- `thread.write` - Write to thread
- `thread.read` - Read thread data

## Permission Categories

### App-Level Permissions

#### Query Execution
```
query.execution.*                    # All contracts
query.execution.<contract_name>      # Specific contract
```

**Logic**: Check if user has `*` wildcard, if not check for specific contract name.

#### Contract Management
```
contract.*                           # All contract operations
contract.create                      # Create contracts
contract.read.*                      # Read all contracts
contract.read.<contract_name>        # Read specific contract
contract.update.*                    # Update all contracts
contract.update.<contract_name>      # Update specific contract
contract.delete.*                    # Delete all contracts
contract.delete.<contract_name>      # Delete specific contract
```

**Note**: `contract.read.*` is granted to all roles by default.

#### API Key Management
```
apikey.*                             # All API key operations
apikey.create
apikey.read
apikey.update
apikey.delete
```

### Runtime-Level Permissions

#### Notification Permissions
```
notification.violations.*            # All violations
notification.violations.critical     # System-wide critical violations
notification.violations.own          # Only violations on own steps
notification.completions.*           # All step completions
notification.completions.own         # Only own step completions
notification.failed.*                # All failure notifications
notification.failed.own              # Only own step failures
notification.thread.lifecycle        # Thread lifecycle events
```

#### Thread Permissions
```
thread.*                             # All thread operations
thread.create
thread.read
thread.write
thread.delete
```

## Default Roles

### Runtime-Level Roles

#### Owner
**Scope**: `runtime_level`
**Use Case**: Thread creator with full control

**Permissions**:
```yaml
- violations.*
- completions.*
- thread.lifecycle
- thread.create
- thread.write
- thread.read
```

#### Participant
**Scope**: `runtime_level`
**Use Case**: Active party in thread (contract-defined role)

**Permissions**:
```yaml
- violations.critical          # System-wide critical violations
- violations.own               # Only violations on their own steps
- failed.own
- completions.own              # All step completions
- thread.lifecycle
- thread.write
- thread.read
```

#### External
**Scope**: `runtime_level`
**Use Case**: External participant with limited access

**Permissions**:
```yaml
- completions.own
- failed.own
- thread.write
- thread.read
```

#### Observer
**Scope**: `runtime_level`
**Use Case**: Read-only monitoring access

**Permissions**:
```yaml
- thread.read
```

### App-Level Roles

#### Reader
**Scope**: `app_level`
**Use Case**: Read-only API access

**Permissions**:
```yaml
- thread.read
- query.execution.*
```

#### Standard
**Scope**: `app_level`
**Use Case**: Standard API user

**Permissions**:
```yaml
- thread.read
- thread.write
- query.execution.*
```

#### Account
**Scope**: `app_level`
**Use Case**: Full account management

**Permissions**:
```yaml
- apikey.*
- contract.*
- query.execution.*
```

### Service Account Roles

#### standard_service
**Scope**: `app_level`
**Use Case**: Default service account role

**Permissions**: Similar to standard user role

#### reader
**Scope**: `app_level`
**Use Case**: Read-only service access

**Permissions**: Similar to reader user role

## Notification Scope Resolution

Notification scope determines what notifications a user/service account receives. It's resolved in this **priority order**:

### 1. Creator (Highest Priority)
```go
if isCreator {
    return "owner", nil  // Always owner scope
}
```

### 2. Explicit Scope
Passed via invitation token or direct join:
```javascript
const token = await thread.inviteParty({
  role: 'logistics',
  notificationScope: 'observer'  // Explicit override
});
```

### 3. System default_scope (Lowest Priority)
Global fallback comes from the Engine's `config.yaml` under `notification_system.default_scope`.

## Invitation System

### Current Implementation

```javascript
// SDK inviteParty (notification scope inferred)
const token = await thread.inviteParty({
  role: 'logistics',              // Thread role (what steps they can execute)
  permissions: 'read,write',      // Thread permissions
  expiresIn: '48h'                // Token expiry
});
// Notification scope is inferred from role via contract role_defaults
```

### Future Enhancement (Explicit Notification Scope)

```javascript
const token = await thread.inviteParty({
  role: 'logistics',              // Thread role (execution permissions)
  permissions: 'read,write',      // Thread permissions
  notificationScope: 'observer',  // Notification filtering (independent)
  expiresIn: '48h'
});
```

**Benefits**:
- ✅ Clear separation: Thread role ≠ Notification preferences
- ✅ Flexible: Can have write access but minimal notifications
- ✅ Backward compatible: Defaults to role's notification scope if not specified

## Authorization Flow

### Two-Layer Authorization

```
Request → API Layer → Thread Layer → Notification Layer
           ↓            ↓              ↓
        app_level   runtime_level   notification_scope
```

### Example Scenarios

#### Scenario 1: Read-Only Account with Owner Thread Role

**User**: `read_only` account scope + `owner` thread role

**Results**:
- ✅ Can view threads via API (read_only allows this)
- ❌ Cannot create contracts via API (read_only doesn't allow this)
- ✅ Has full control within assigned threads (owner role)
- ✅ Receives all notifications for their threads (owner scope)

#### Scenario 2: Standard Account with Participant Thread Role

**User**: `standard` account scope + `participant` thread role

**Results**:
- ✅ Can read/write threads via API
- ✅ Can execute GraphQL queries
- ✅ Can write steps in assigned threads
- ✅ Receives own violations + critical violations + all completions

#### Scenario 3: Service Account with Reader Role

**Service Account**: `reader` role

**Results**:
- ✅ Can read threads via API
- ✅ Can execute GraphQL queries
- ❌ Cannot create/update contracts
- ❌ Cannot create API keys
- Thread participation depends on invitation

## Configuration Files

### RBAC Configuration
- **Location**: `/threadify-go/shared/rbac/`
- **Files**:
  - `roles.json` - Role definitions with permissions
  - `permissions.json` - Permission definitions

### Contract Configuration
- **Location**: Gherkin `.feature` contracts
- **Defines**:
  - Thread-specific role mappings
  - Step owners and business context

### System Configuration
- **Location**: `/threadify-go/config/config.yaml`
- **Defines**:
  - Default notification scope
  - Notification system scopes
  - Invitation defaults

## Implementation Details

### Backend Middleware

#### Contract JWT Middleware
```go
// Location: /threadify-go/internal/middleware/
contracts.Use(middleware.ContractJWTMiddleware(jwtValidator))
```

Validates JWT token and extracts user claims.

#### Contract RBAC Middleware
```go
contracts.GET("",
    middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"),
    contractHandler.GetAllContracts)
```

Checks if user has required permission.

### Permission Checking Logic

```go
// Wildcard check
if hasPermission(user, "contract.read.*") {
    return true
}

// Specific resource check
if hasPermission(user, "contract.read.product_delivery") {
    return true
}

return false
```

### Notification Filtering

```go
// Location: /threadify-go/internal/service/scope_resolver.go

func ShouldReceiveNotification(
    scope string,
    notificationType string,
    stepOwner string,
    userRole string,
    severity string,
) bool {
    switch scope {
    case "owner":
        return true  // Owner sees everything
    
    case "participant":
        if notificationType == "completion" {
            return true
        }
        if notificationType == "violation" {
            if severity == "critical" {
                return true  // Critical violations anywhere
            }
            if stepOwner == userRole {
                return true  // Own step violations
            }
        }
        return false
    
    case "observer":
        return notificationType == "completion"
    
    default:
        return false
    }
}
```

## Migration from Old System

### Old System (Scopes)
- Used single "scope" field: `owner`, `participant`, `observer`, `admin`, `developer`, `ci_cd`
- Mixed API access and thread permissions
- No clear separation of concerns

### New System (Roles + Permissions)
- Separate roles for different contexts
- Explicit permissions per role
- Clear separation:
  - Service account roles (app-level access)
  - Thread participant roles (runtime permissions)
  - Notification scopes (notification filtering)

### Migration Steps
1. ✅ Updated database schema to use `role` instead of `scope`
2. ✅ Created RBAC configuration files
3. ✅ Implemented middleware for permission checking
4. ✅ Updated frontend to use role-based UI
5. ✅ Migrated existing service accounts to new role system

## Best Practices

### 1. Principle of Least Privilege
Grant minimum permissions required for the task.

### 2. Role Assignment
- Use `standard_service` for most service accounts
- Use `reader` for monitoring/analytics services
- Use `account` role only for admin users

### 3. Thread Participation
- Assign `owner` role to thread creator
- Assign `participant` role to active parties
- Assign `observer` role for monitoring/auditing

### 4. Notification Scopes
- Use `owner` scope sparingly (high notification volume)
- Use `participant` scope for most active parties
- Use `observer` scope for monitoring dashboards

### 5. Contract Design
Define clear role_defaults in contracts:
```yaml
notification_config:
  role_defaults:
    merchant: "owner"
    logistics: "participant"
    customer: "external"
    auditor: "observer"
  default_scope: "participant"
```

## Security Considerations

1. **Never trust client-side permission checks** - Always validate on server
2. **Use JWT for authentication** - Validate on every request
3. **Check permissions at multiple layers** - API + Thread + Notification
4. **Audit permission changes** - Log all role/permission modifications
5. **Regular permission reviews** - Ensure users have appropriate access

---

*Last Updated: January 27, 2026 - Complete permission and role model documentation*

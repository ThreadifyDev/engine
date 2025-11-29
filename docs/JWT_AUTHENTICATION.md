# JWT Authentication Implementation

This document explains the JWT authentication system implemented in ThreadifyEngine.

## Overview

The application now supports JWT (JSON Web Token) bearer token authentication using the JJWT library. The authentication system validates tokens on protected routes and makes user information available to route handlers.

## Architecture

### 1. **Dependencies Added**

**File: `gradle/libs.versions.toml`**
- `jwt = "0.12.6"` - JJWT library version
- `jjwt-api` - JWT API interfaces
- `jjwt-impl` - JWT implementation
- `jjwt-jackson` - JSON processing for JWT
- `ktor-server-auth` - Ktor authentication plugin
- `ktor-server-auth-jwt` - Ktor JWT support

**Technical Details:**
- JJWT 0.12.6 uses modern API (not deprecated methods)
- HMAC-SHA256 algorithm for token signing
- Jackson for JSON serialization of claims

### 2. **Configuration (application.yaml)**

```yaml
jwt:
    secret: "$JWT_SECRET:your-256-bit-secret-key..."
    issuer: "$JWT_ISSUER:threadify-engine"
    audience: "$JWT_AUDIENCE:threadify-users"
    realm: "$JWT_REALM:Threadify API"
    expirationMs: "$JWT_EXPIRATION_MS:3600000"  # 1 hour
```

**Technical Details:**
- Environment variables with fallback defaults (format: `$VAR:default`)
- Secret must be at least 32 characters for HMAC-SHA256
- Expiration in milliseconds (3600000 = 1 hour)
- Issuer/Audience for token validation

### 3. **Authentication Service**

**File: `src/main/kotlin/Services/Authentication.kt`**

The service handles all JWT operations:

#### `createToken(userId: String, claims: Map<String, Any>): String`
- Creates a signed JWT token with user ID and custom claims
- Sets standard claims: subject, issuer, audience, issuedAt, expiration
- Signs with HMAC-SHA256 using the secret key
- Returns compact JWT string (header.payload.signature)

**Technical Flow:**
1. Generate current timestamp and expiration date
2. Build JWT with standard claims (subject, issuer, audience, etc.)
3. Add custom claims (roles, permissions, metadata)
4. Sign with HMAC-SHA256 using secret key
5. Return base64-encoded token string

#### `verifyToken(token: String): Claims?`
- Parses and validates JWT token
- Verifies signature using secret key
- Checks issuer and audience match configuration
- Validates expiration time
- Returns Claims object or null if invalid

**Technical Flow:**
1. Parse JWT string into header, payload, signature
2. Verify signature matches (prevents tampering)
3. Validate issuer claim matches expected value
4. Validate audience claim matches expected value
5. Check expiration hasn't passed
6. Return claims if all checks pass, null otherwise

#### `getUserIdFromToken(token: String): String?`
- Extracts user ID (subject claim) from token
- Convenience method wrapping verifyToken

#### `isTokenValid(token: String): Boolean`
- Quick validation check without extracting claims

### 4. **Global Services Singleton**

**File: `src/main/kotlin/Utilities/GlobalServices.kt`**

```kotlin
object GlobalServices {
    lateinit var authenticationService: Authentication
}
```

**Technical Details:**
- Kotlin `object` provides thread-safe singleton
- `lateinit` allows initialization after object creation
- Accessed throughout app: `GlobalServices.authenticationService`
- Initialized in `loadServices()` before middleware setup

### 5. **Service Initialization**

**File: `src/main/kotlin/Services.kt`**

```kotlin
fun Application.loadServices() {
    val jwtSecret = environment.config.property("jwt.secret").getString()
    val jwtIssuer = environment.config.property("jwt.issuer").getString()
    val jwtAudience = environment.config.property("jwt.audience").getString()
    val jwtExpirationMs = environment.config.property("jwt.expirationMs").getString().toLong()
    
    GlobalServices.authenticationService = Authentication(
        secret = jwtSecret,
        issuer = jwtIssuer,
        audience = jwtAudience,
        expirationMs = jwtExpirationMs
    )
}
```

**Technical Details:**
- Reads JWT config from application.yaml
- Creates Authentication service instance
- Stores in GlobalServices for app-wide access
- Must run before `configureAuthentication()`

### 6. **Authentication Middleware**

**File: `src/main/kotlin/AuthenticationConfig.kt`**

```kotlin
fun Application.configureAuthentication() {
    install(Authentication) {
        bearer("auth-jwt") {
            realm = jwtRealm
            authenticate { tokenCredential ->
                val token = tokenCredential.token
                val authService = GlobalServices.authenticationService
                val claims = authService.verifyToken(token)
                
                if (claims != null) {
                    UserIdPrincipal(claims.subject)
                } else {
                    null  // Returns 401 Unauthorized
                }
            }
        }
    }
}
```

**Technical Details:**
- Ktor's `Authentication` plugin with `bearer` provider
- Extracts token from `Authorization: Bearer <token>` header
- Validates token using Authentication service
- Creates `UserIdPrincipal` on success (available in routes)
- Returns null on failure (Ktor sends 401 Unauthorized)

**Extension Functions:**
- `call.getUserId()` - Get authenticated user ID
- `call.getToken()` - Get raw JWT token string
- `call.getClaim(name)` - Extract custom claim from token

### 7. **Application Module Setup**

**File: `src/main/kotlin/Application.kt`**

```kotlin
fun Application.module() {
    loadServices()              // 1. Initialize services first
    configureMonitoring()
    configureAdministration()
    configureSockets()
    configureSerialization()
    configureDatabases()
    configureAuthentication()   // 2. Setup auth middleware
    configureRateLimiting()
    configureHTTP()
    configureRouting()
}
```

**Critical Order:**
1. `loadServices()` must run first to initialize Authentication service
2. `configureAuthentication()` sets up middleware (depends on service)
3. Other configuration can run in any order

## Usage in Routes

### Protected Routes

**File: `src/main/kotlin/Routers/Contracts.kt`**

```kotlin
fun Route.contracts() {
    // Public route - no auth required
    post("/contracts/login") {
        val userId = call.receiveText()
        val authService = GlobalServices.authenticationService
        
        val token = authService.createToken(
            userId = userId,
            claims = mapOf(
                "role" to "user",
                "permissions" to listOf("read", "write")
            )
        )
        
        call.respond(mapOf("token" to token))
    }
    
    // Protected routes - require JWT
    authenticate("auth-jwt") {
        get("/contracts") {
            val userId = call.getUserId()  // Get authenticated user
            val role = call.getClaim("role")  // Get custom claim
            
            call.respond(mapOf(
                "message" to "Contracts list",
                "user" to userId,
                "role" to role
            ))
        }
    }
}
```

**Technical Flow:**
1. Client calls `/v1/contracts/login` with user ID
2. Server creates JWT with user ID and custom claims
3. Client receives token
4. Client includes token in subsequent requests: `Authorization: Bearer <token>`
5. Middleware validates token before route handler executes
6. Route handler accesses user info via `call.getUserId()`

## Testing

### Example: Get JWT Token

```bash
# Get a token
curl -X POST http://localhost:8080/v1/contracts/login \
  -H "Content-Type: text/plain" \
  -d "user123"

# Response:
{
  "token": "eyJhbGciOiJIUzI1NiJ9...",
  "userId": "user123",
  "message": "Use this token in Authorization header as: Bearer <token>"
}
```

### Example: Access Protected Route

```bash
# Without token - 401 Unauthorized
curl http://localhost:8080/v1/contracts

# With valid token - 200 OK
curl http://localhost:8080/v1/contracts \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9..."

# Response:
{
  "message": "Contracts list",
  "authenticatedUser": "user123",
  "userRole": "user"
}
```

## Security Considerations

1. **Secret Key**: Use a strong, randomly generated secret (min 32 chars)
2. **HTTPS**: Always use HTTPS in production to prevent token interception
3. **Token Expiration**: Tokens expire after 1 hour (configurable)
4. **Token Storage**: Clients should store tokens securely (not localStorage)
5. **Signature Verification**: All tokens are cryptographically verified
6. **Claims Validation**: Issuer and audience are validated on every request

## Token Structure

A JWT consists of three parts separated by dots:

```
header.payload.signature
```

**Header:**
```json
{
  "alg": "HS256",
  "typ": "JWT"
}
```

**Payload (Claims):**
```json
{
  "sub": "user123",
  "iss": "threadify-engine",
  "aud": "threadify-users",
  "iat": 1701234567,
  "exp": 1701238167,
  "role": "user",
  "permissions": ["read", "write"]
}
```

**Signature:**
```
HMACSHA256(
  base64UrlEncode(header) + "." + base64UrlEncode(payload),
  secret
)
```

## Error Handling

- **No token provided**: 401 Unauthorized
- **Invalid token format**: 401 Unauthorized
- **Expired token**: 401 Unauthorized (caught in verifyToken)
- **Invalid signature**: 401 Unauthorized (token tampered with)
- **Wrong issuer/audience**: 401 Unauthorized

## Integration with Other Services

The Authentication service can be accessed anywhere in the application:

```kotlin
val authService = GlobalServices.authenticationService

// Create token
val token = authService.createToken("userId", mapOf("role" to "admin"))

// Verify token
val claims = authService.verifyToken(token)

// Check validity
val isValid = authService.isTokenValid(token)

// Extract user ID
val userId = authService.getUserIdFromToken(token)
```

## Future Enhancements

1. **Refresh Tokens**: Implement refresh token mechanism for long-lived sessions
2. **Token Revocation**: Add Redis-based token blacklist
3. **Role-Based Access Control**: Create route guards based on roles
4. **Multi-Factor Authentication**: Add MFA support
5. **OAuth2 Integration**: Support external identity providers

# Technical Debt & TODO Items

## High Priority

### 1. PostgreSQL NOTIFY/LISTEN for First Instrumentation Detection
**Location:** Getting-started page  
**Current State:** Manual button click to proceed to dashboard  
**Desired State:** Automatic detection when user creates their first thread  
**Implementation:**
- Set up PostgreSQL NOTIFY trigger on thread creation
- Backend listens for notifications
- Update `first_instrumentation_done` flag automatically
- Push real-time update to frontend via WebSocket
- Auto-redirect user to dashboard

**Files to modify:**
- `/api/migrations/` - Add trigger for thread creation
- Backend service to listen for NOTIFY events
- `/app/routes/getting-started.tsx` - Remove manual button, add real-time listener

---

### 2. React useEffect Warnings
**Location:** `/app/routes/dashboard.tsx:30`  
**Issue:** Calling `setState` synchronously within useEffect causes cascading renders  
**Fix:** Move state initialization outside of useEffect or use proper data fetching pattern

```typescript
// Current (problematic):
useEffect(() => {
  const storedUser = api.getStoredUser();
  setUser(storedUser); // ❌ Causes cascading renders
}, [navigate]);

// Better approach:
const [user] = useState(() => api.getStoredUser()); // ✅ Initialize once
```

---

### 3. Missing useEffect Dependencies
**Location:** `/app/routes/getting-started.tsx`  
**Issue:** `fetchCodeSamples` function not included in dependency array  
**Fix:** Either include it or wrap in useCallback

---

## Medium Priority

### 4. Code Samples File Path
**Current:** Relative path `./code_samples` works but fragile  
**Better:** Use absolute path or environment variable  
**Files:** `/api/cmd/server/main.go:78`

---

### 5. API Key Security
**Current:** API key stored in sessionStorage (cleared after use)  
**Consider:** 
- Add warning about not committing API keys to version control
- Implement API key rotation
- Add usage analytics per API key

---

### 6. Error Handling Improvements
**Location:** Multiple handlers in `/api/internal/handlers/`  
**Current:** Generic error messages  
**Better:** 
- Structured error responses with error codes
- Client-friendly error messages
- Detailed logging for debugging

---

### 7. Migration Rollback Issues
**Location:** `/api/migrations/000002_update_otp_code_length.down.sql`  
**Issue:** SQL syntax errors in down migration  
**Fix:** Correct the ALTER TABLE syntax for PostgreSQL

---

## Low Priority

### 8. Markdown Linting
**Location:** `/Claude.md`  
**Issues:**
- Multiple consecutive blank lines
- Trailing spaces
**Fix:** Run markdown formatter

---

### 9. Code Sample Language Support
**Current:** JavaScript, Python, Go  
**Future:** Add more languages (Ruby, Java, C#, PHP, etc.)

---

### 10. Getting-Started Page Enhancements
**Potential improvements:**
- Add video tutorial
- Interactive code playground
- Step-by-step wizard
- Link to full documentation

---

## Architecture Improvements

### 11. Frontend State Management
**Current:** localStorage + React useState  
**Consider:** 
- Context API for global state
- React Query for server state
- Zustand or Redux for complex state

---

### 12. API Client Type Safety
**Current:** Manual type definitions  
**Better:** 
- Generate TypeScript types from OpenAPI spec
- Use tRPC for end-to-end type safety
- Add runtime validation with Zod

---

### 13. Testing
**Missing:**
- Unit tests for handlers
- Integration tests for API endpoints
- E2E tests for onboarding flow
- Frontend component tests

---

### 14. Authentication Improvements
**Current:** JWT in localStorage  
**Consider:**
- HttpOnly cookies for better security
- Refresh token rotation
- Session management
- Rate limiting per user

---

### 15. Database Connection Pooling
**Review:** Ensure proper connection pool configuration for production load

---

## Documentation Needs

### 16. API Documentation
- OpenAPI/Swagger spec
- Postman collection
- API versioning strategy

---

### 17. Developer Onboarding
- Setup guide
- Architecture documentation
- Contributing guidelines
- Code style guide

---

### 18. Deployment Documentation
- Production deployment guide
- Environment variables reference
- Monitoring and logging setup
- Backup and recovery procedures

---

## Performance Optimizations

### 19. Frontend Bundle Size
- Code splitting
- Lazy loading routes
- Tree shaking unused code
- Image optimization

---

### 20. Database Indexing
- Review query patterns
- Add appropriate indexes
- Query performance monitoring

---

## Security Hardening

### 21. Input Validation
- Sanitize all user inputs
- Validate email formats
- Password strength requirements
- SQL injection prevention audit

---

### 22. CORS Configuration
**Current:** Trusts all proxies (warning in logs)  
**Fix:** Configure specific allowed origins for production

---

### 23. Rate Limiting
- Implement per-endpoint rate limits
- Add IP-based rate limiting
- Protect against brute force attacks

---

## Monitoring & Observability

### 24. Logging
- Structured logging
- Log aggregation
- Error tracking (Sentry, etc.)
- Performance monitoring

---

### 25. Metrics
- API response times
- Error rates
- User analytics
- Database query performance

---

## Future Features (From Claude.md)

### 26. Phase 2.1: Contract Management
- Create/edit contracts
- Version management
- Contract validation

### 27. Phase 2.2: Thread Monitoring
- Real-time thread visualization
- Thread search and filtering
- Thread analytics

### 28. Phase 2.4: Team Management
- Invite team members
- Role-based access control
- Team settings

### 29. Phase 2.5: Settings & Profile
- User profile management
- Company settings
- Notification preferences
- API key management (already done)

---

## Notes

- Items marked with ❌ are blocking issues
- Items marked with ⚠️ need attention soon
- Items marked with 💡 are nice-to-have improvements

**Last Updated:** January 24, 2026

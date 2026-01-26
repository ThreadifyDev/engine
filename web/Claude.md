Threadify enables you build systems that understand your business workflow.
Threadify creates context graph for your business workflow by instrumenting business processes from any source.

Sources can be Agents, apps, human in the loop, and even frontend clients.
Threads are live task records that contains all services, agents, and humans that participated in the delivery of a business service.

e.g A user placed an order on an e-commerce platform. FRom the moment they click purchase, you could start instrumenting as the order request goes from ordered to delivered. You could also breakdown the workflow into smaller ones like payment service, order fulfillment, delivery and logistics, etc.

In threadify there is something called contract. Contracts are the rules that define your business flow in your system. Considering to delvier a business service you might have multiple workflows of themselves that operate to deliver the service. Contracts/Rulebooks define the rules for each workflow.

Then when you create a thread, you can assign a contract to it. Then as you instrument into the thread, threadify would validate the steps/events against the contract and identify if all is well or if there is a violation. Threadify then uses websocket to send you events that you listen to and take action based on the events. This stands out because apart from just knowing when something is wrong, you also get the full context regardless of how many apps/actions was involved to get the delivery of the service to this point.

*PROJECT DETAILS:*
The application must follow a black and white theme with Block font
We are using Remix with golang for backend (using gin).
Postges for DB = URL ("postgres://td_engine:tdtdtd@localhost:5434/threadify?sslmode=disable")




**The 3 main components of Threadify are:**
1. Threadify Engine (the main engine that manages the threads and contracts) 
2. Threadify Web (which is this application, that has the UI and Backend API for users to be able to manage and view threads and contracts)
3. Threadify SDK: (https://www.npmjs.com/package/@threadify/sdk)

**Threadify Web has these components:**
1. *Contract & Contract Versions* - When you create a contract a version is created (v1) and when you update the contract a new version is created (v2). Contract versions can be deleted. Contracts is a yaml script (check contract.yaml for example). When a thread is created, if no version is specified, it binds to the latest version. Contracts also define notification configuration for different actions (who can receive what notification), but the actual reaction handlers are implemented in application code via the SDK.

2. *Threads* - Threads are live grouped records that contains all services, agents, and humans that participated in the delivery of a business service. You cannot create a thread from the UI. You can view threads that has been created and it renders as a flowchart(mermaid maybe). You can also view as a thread tree and view more details of each step of the thread when you click on them. It opens a sidebar that shows the details of the step (with a button in the sidebar to view context of the step). Because threads represent a records of actions in the delivery of a business service, a medium size business could have hundreds of them or even thousands. This means it's not important to view all threads, instead thread should mostly be shown after a search operation and shows only 20 threads at a time. Threads are meant to be used to enable understanding of the delivery of business services. A thread is then important for context of all that was executed in the delivery of the business service, if they followed the pattern we wanted, and ability to react to changes in a thread (reactions could be circuit breaking, smart routing/agentic routing, workflow coordination, etc).

3. *Steps* - Steps are the events that are sent to threadify engine to validate against the contract. Steps are sent from the SDK. Steps are grouped into threads. Steps are also used to react to changes in a thread (reactions could be circuit breaking, smart routing/agentic routing, workflow coordination, etc). Steps are the basic unit of work in threadify. A step can have sub-steps, but sub-steps cannot have sub-steps (max nesting depth = 1).

4. *GraphQL builder* - support for devs/teams to build GraphQL queries and save them to be used later. Think CRUD. Saved queries are company-scoped by default (only visible within the company). Queries can be marked as "public" to make them available to all users. Queries can be updated and deleted (only if you have the permission to or you're the creator).

5. *Permissions* - support for permissions to be able to control who can do what. The hierarchy is: User → Roles → Permissions (and separately: ServiceAccount → Roles → Permissions). Users can only be assigned certain roles (role assignment is restricted). Permissions control what users can see in the UI and what they can do in the API.

6. *API Keys* - support for API keys to be able to control who can do what. API keys are also used to control who can do what in the API. API Keys are used to interact with the SDK and can be mapped to either a User OR a Service Account (these are separate entities). API Keys inherit permissions from the associated User or Service Account.
   - **Security**: We do NOT store the raw API key. Only the hash is stored in the database. The raw key is shown to the user ONCE at creation time and cannot be retrieved again.
   - **Service Account Creation**: When a new API Key is created (not renewed), a Service Account is automatically created and linked to it. When an API Key is renewed/rotated, the existing Service Account is reused.

7. *Service Accounts* - Non-human identities for backend applications/services. Service Accounts are separate from Users (a User cannot be a Service Account). Service Accounts follow the same permission model: ServiceAccount → Roles → Permissions. They can have API Keys mapped to them. Used for machine-to-machine authentication.

8. *Reactions* - Reactions (circuit breaking, smart routing, workflow coordination, etc.) are implemented in application code, not in the UI. The contract defines notification configuration (who receives what notification for which actions), and the SDK provides handlers (onViolation, onCompleted, onFailed, etc.) for reacting to these notifications in code.


**USER FLOW:**

1. A simple login system with email and password.
2. A signup page with Company name, email and password.
3. Send OTP via email using Plunk.
4. Onboarding page after verifying OTP.
5. First onboarding form asks for company size, company's industry, what they want to use Threadify for.
6. Second onboarding form asks for full name, job role. Then the form is sent to the backend and an APIKey is created for the user and user is taken to the dashboard.
7. But instead of the main dashboard, it would be a page telling the user they need to atleast make one instrumentation before they can start using Threadify. It can have a title of "We've created an APIKey so you can get started with Threadify"
8. In that page, we show the user the apiKey and a button to copy it. This is the ONLY time the raw API key is visible - it cannot be retrieved later since we only store the hash.
9. We should also show the user a simple code sample for connecting and instrumenting a thread. The code would be returned from the backend in a {"<language>": "<code that would be stored in a JSON file>"} so that the more languages we support, the more code samples we can show.
10. The API that returns the code sample would take a param called "codeType" which allows us determine what sample code we are trying to fetch if it's pure instrumenting, a sample code that uses contracts, or a sample code that uses contracts and reactions, etc.
11. After the user has done their first instrumenting, we detect this using PostgreSQL NOTIFY/LISTEN on the threads table. When a thread is created for the user's company, the backend receives the notification and can update the user's onboarding status.
12. User is then redirected to the main dashboard.


**IMPLEMENTATION GUIDELINES:**

Follow these rules when implementing this application:

1. *Security*
   - Never store raw API keys - only store hashed values (use bcrypt or similar)
   - Never log API keys or sensitive credentials
   - Use parameterized queries to prevent SQL injection
   - Validate all user input on both frontend and backend
   - Use HTTPS for all API calls
   - Implement proper CORS configuration

2. *Database*
   - Use migrations for all schema changes
   - Use transactions for operations that modify multiple tables
   - Implement proper indexing for frequently queried columns
   - Use PostgreSQL NOTIFY/LISTEN for real-time updates (e.g., first instrumentation detection)

3. *API Design*
   - Follow RESTful conventions for the Go/Gin backend
   - Use proper HTTP status codes (201 for created, 400 for bad request, 401 for unauthorized, 403 for forbidden, 404 for not found)
   - Return consistent error response format: `{"error": {"code": "ERROR_CODE", "message": "Human readable message"}}`
   - Paginate list endpoints (default 20 items per page)

4. *Frontend (Remix)*
   - Use Remix loaders for data fetching (SSR)
   - Use Remix actions for form submissions
   - Implement proper loading and error states
   - Follow the black and white theme with Block font consistently
   - Use proper form validation with clear error messages

5. *Code Style*
   - Use TypeScript for frontend code
   - Use Go modules and proper package structure for backend
   - Write descriptive variable and function names
   - Add comments for complex business logic
   - Keep functions small and focused (single responsibility)

6. *Authentication & Authorization*
   - Use JWT for session management
   - Implement proper token refresh mechanism
   - Check permissions on every protected endpoint
   - Never trust client-side permission checks alone

7. *Error Handling*
   - Never expose internal error details to users
   - Log errors with sufficient context for debugging
   - Implement graceful degradation where possible
   - Show user-friendly error messages

8. *Testing*
   - Write unit tests for business logic
   - Write integration tests for API endpoints
   - Test edge cases and error scenarios 


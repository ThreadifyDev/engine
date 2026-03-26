package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	mcp "github.com/metoro-io/mcp-golang"
	mcptransport "github.com/metoro-io/mcp-golang/transport/http"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
	"go.uber.org/zap"
)

type contextKeyAPIKey struct{}

// pullContextAuthMiddleware lifts the X-API-Key from the Gin context into the
// request context so it is accessible inside MCP tool handlers.
func pullContextAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		ctx := context.WithValue(c.Request.Context(), contextKeyAPIKey{}, apiKey)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// graphQLQuery executes a GraphQL query against the local engine endpoint and
// returns the result as a JSON string wrapped in an MCP ToolResponse.
// The X-API-Key is forwarded from the context so auth is preserved end-to-end.
func graphQLQuery(ctx context.Context, apiPort int, cfg *config.Config, planSvc interfaces.PlanService, logger *zap.Logger, query string, variables map[string]interface{}) (*mcp.ToolResponse, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx, "POST",
		fmt.Sprintf("http://localhost:%d/graphql", apiPort),
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	// GinTransport.Handler() creates a fresh context.Background() and embeds
	// the Gin context under the "ginContext" key — extract the API key from there.
	// We also fall back to our own contextKeyAPIKey for any non-Gin callers.
	apiKey := ""
	if ginCtx, ok := ctx.Value("ginContext").(*gin.Context); ok && ginCtx != nil {
		apiKey = ginCtx.GetHeader("X-API-Key")
	}
	if apiKey == "" {
		apiKey, _ = ctx.Value(contextKeyAPIKey{}).(string)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GraphQL error (status %d): %s", resp.StatusCode, string(body))
	}

	// Return the raw JSON as text content — LLMs read this fine
	return mcp.NewToolResponse(mcp.NewTextContent(string(body))), nil
}

func mustRegister(name string, err error, logger *zap.Logger) {
	if err != nil {
		logger.Fatal("failed to register MCP tool", zap.String("tool", name), zap.Error(err))
	}
}

func mountMCPServer(r *gin.RouterGroup, cfg *config.Config, planSvc interfaces.PlanService, logger *zap.Logger) {
	mcpTransport := mcptransport.NewGinTransport()
	mcpSrv := mcp.NewServer(mcpTransport)
	port := cfg.Server.Port

	// Threadify MCP Server
	// Execution graph infrastructure for distributed business processes.
	// Catches silent failures where services return 200 but downstream processes never run.
	// Example: payment authorized but fulfillment never triggered, fraud check skipped, etc.

	// 1. get_thread
	type GetThreadArgs struct {
		ID string `json:"id" jsonschema_description:"The UUID of the thread"`
	}
	mustRegister("get_thread", mcpSrv.RegisterTool("get_thread", "Get a thread by ID with its steps", func(ctx context.Context, args GetThreadArgs) (*mcp.ToolResponse, error) {
		return graphQLQuery(ctx, port, cfg, planSvc, logger,
			`query ($id: ID!) { thread(id: $id) { id contractId contractName status startedAt completedAt error steps { stepName status } } }`,
			map[string]interface{}{"id": args.ID},
		)
	}), logger)

	// 2. search_threads
	type SearchThreadsArgs struct {
		ContractName    string `json:"contractName,omitempty" jsonschema_description:"Filter by contract name"`
		ContractVersion int    `json:"contractVersion,omitempty" jsonschema_description:"Filter by contract version"`
		Status          string `json:"status,omitempty" jsonschema_description:"Filter by status: ACTIVE, COMPLETED, FAILED"`
		Limit           int    `json:"limit,omitempty" jsonschema_description:"Maximum number of results (default 50)"`
	}
	mustRegister("search_threads", mcpSrv.RegisterTool("search_threads", "Search threads with optional filters", func(ctx context.Context, args SearchThreadsArgs) (*mcp.ToolResponse, error) {
		vars := map[string]interface{}{}
		if args.ContractName != "" {
			vars["contractName"] = args.ContractName
		}
		if args.ContractVersion != 0 {
			vars["contractVersion"] = args.ContractVersion
		}
		if args.Status != "" {
			vars["status"] = args.Status
		}
		if args.Limit > 0 {
			vars["limit"] = args.Limit
		} else {
			vars["limit"] = 50
		}
		return graphQLQuery(ctx, port, cfg, planSvc, logger,
			`query ($contractName: String, $contractVersion: Int, $status: String, $limit: Int) {
				threads(contractName: $contractName, contractVersion: $contractVersion, status: $status, limit: $limit) {
					threads { id status contractName contractVersion startedAt completedAt }
				}
			}`,
			vars,
		)
	}), logger)

	// 3. contract_graph
	type ContractGraphArgs struct {
		Name    string `json:"name" jsonschema_description:"Contract name (required)"`
		Version int    `json:"version,omitempty" jsonschema_description:"Contract version (defaults to latest)"`
	}
	mustRegister("contract_graph", mcpSrv.RegisterTool("contract_graph", "Get the graph structure of a contract", func(ctx context.Context, args ContractGraphArgs) (*mcp.ToolResponse, error) {
		vars := map[string]interface{}{"name": args.Name}
		if args.Version > 0 {
			vars["version"] = args.Version
		}
		return graphQLQuery(ctx, port, cfg, planSvc, logger,
			`query ($name: String!, $version: Int) {
				contractGraph(name: $name, version: $version) {
					parties
					notificationConfig { defaultScope }
					graph { entryPoints terminalSteps nodes { id type required steps next } }
				}
			}`,
			vars,
		)
	}), logger)

	// 4. resolve_actors
	type ResolveActorsArgs struct {
		IDs []string `json:"ids" jsonschema_description:"List of actor UUIDs to resolve to names"`
	}
	mustRegister("resolve_actors", mcpSrv.RegisterTool("resolve_actors", "Resolve actor IDs to names and types", func(ctx context.Context, args ResolveActorsArgs) (*mcp.ToolResponse, error) {
		return graphQLQuery(ctx, port, cfg, planSvc, logger,
			`query ($ids: [String!]!) { resolveActors(ids: $ids) { id name type companyName } }`,
			map[string]interface{}{"ids": args.IDs},
		)
	}), logger)

	// 10. verify_thread_integrity
	type VerifyThreadIntegrityArgs struct {
		ThreadID string `json:"threadId" jsonschema_description:"Thread UUID to verify"`
	}
	mustRegister("verify_thread_integrity", mcpSrv.RegisterTool("verify_thread_integrity", "Verify thread integrity and detect tampering via hash chain validation", func(ctx context.Context, args VerifyThreadIntegrityArgs) (*mcp.ToolResponse, error) {
		return graphQLQuery(ctx, port, cfg, planSvc, logger,
			`query ($threadId: String!) { verifyThreadIntegrity(threadId: $threadId) { verified lastVerifiedAt totalEvents brokenAt error } }`,
			map[string]interface{}{"threadId": args.ThreadID},
		)
	}), logger)

	// 6. graphql_query - Flexible GraphQL query execution
	type GraphQLQueryArgs struct {
		Query     string                 `json:"query" jsonschema_description:"GraphQL query string (required). Use the 'graphql_schema' resource to discover available types and fields."`
		Variables map[string]interface{} `json:"variables,omitempty" jsonschema_description:"Optional query variables as a JSON object"`
	}
	mustRegister("graphql_query", mcpSrv.RegisterTool(
		"graphql_query",
		"Execute custom GraphQL queries for precise data retrieval. Use when specialized tools don't provide exact fields needed. Check 'graphql_schema' resource for available fields.",
		func(ctx context.Context, args GraphQLQueryArgs) (*mcp.ToolResponse, error) {
			if args.Variables == nil {
				args.Variables = map[string]interface{}{}
			}
			return graphQLQuery(ctx, port, cfg, planSvc, logger, args.Query, args.Variables)
		},
	), logger)

	// Register GraphQL schema as a resource
	err := mcpSrv.RegisterResource(
		"graphql://schema",
		"graphql_schema",
		"Threadify GraphQL schema - shows all available queries for investigating business process execution. Threadify tracks distributed workflow execution step-by-step, helping debug silent failures where services succeed but downstream processes never run. Use this schema to discover how to query threads, steps, validation results, and cryptographic integrity verification.",
		"application/graphql",
		func() (*mcp.ResourceResponse, error) {
			// Read the schema file
			schemaPath := "internal/graphql/schema.graphql"
			content, err := os.ReadFile(schemaPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read schema: %w", err)
			}
			return mcp.NewResourceResponse(
				mcp.NewTextEmbeddedResource("graphql://schema", string(content), "application/graphql"),
			), nil
		},
	)
	if err != nil {
		logger.Fatal("failed to register graphql_schema resource", zap.Error(err))
	}

	// Register prompts to guide LLMs on using Threadify
	type EmptyArgs struct{}

	// Prompt 1: Debug silent failure
	err = mcpSrv.RegisterPrompt(
		"debug_silent_failure",
		"Investigate why a business process failed silently (service returned 200 but downstream steps never ran). Example: payment authorized but order never fulfilled.",
		func(args EmptyArgs) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse(
				"Debug Silent Failure in Threadify",
				mcp.NewPromptMessage(
					mcp.NewTextContent(`You are debugging a silent failure in a distributed business process using Threadify.

**What is Threadify?**
Threadify turns customer requests into live execution graphs. Every customer request is a Thread flowing through your system. It catches failures that traditional monitoring (Grafana, CloudWatch, Datadog) misses - cases where services return 200 but downstream processes never run.

**Core Concepts:**
- **Thread**: One customer request/workflow (e.g., order processing, payment flow) with unique ID, status, and steps
- **Step**: One action in the workflow (e.g., validate_cart, charge_payment) with status, context, and execution history
- **Step Statuses**: success (completed), failed (business logic failure), error (system/technical error)
- **Contract**: Workflow definition with valid state transitions and validation rules

**Common Silent Failure Scenarios:**
- Payment authorized but fulfillment never triggered
- Fraud check skipped under race condition
- Partner integration received event but never acknowledged
- Compliance step ran out of order
- Service returned 200 but downstream step never recorded

**Investigation Workflow:**
1. **Identify the thread**: Ask user for thread ID, customer ID, order ID, or contract name
2. **Get thread details**: Use 'get_thread' to see which steps executed
3. **Compare to expected flow**: Use 'contract_graph' to see what SHOULD have happened
4. **Find the gap**: Identify which expected steps never ran
5. **Check integrity**: Use 'verify_thread_integrity' to detect tampering or missing steps
6. **Analyze context**: Use 'graphql_query' to get detailed step history, error messages, and actor information

**Key Questions to Answer:**
- Which steps executed successfully?
- Which expected steps never ran?
- What was the last successful step before the failure?
- Were there any validation violations?
- Is the hash chain intact (no tampering)?
- What error messages or context are available?

**Example Queries:**
- Find by customer: search_threads with filters, or graphql_query with threadsByRef
- Get execution details: get_thread with step history
- Check expected flow: contract_graph to see valid transitions
- Verify integrity: verify_thread_integrity for cryptographic validation

Start by asking the user for identifying information (thread ID, customer ID, order ID, or contract name).`),
					mcp.RoleUser,
				),
			), nil
		},
	)
	if err != nil {
		logger.Fatal("failed to register debug_silent_failure prompt", zap.Error(err))
	}

	// Prompt 2: Verify process integrity
	err = mcpSrv.RegisterPrompt(
		"verify_process_integrity",
		"Verify that a business process executed correctly with no tampering, missing steps, or out-of-order execution. Uses cryptographic hash chain validation.",
		func(args EmptyArgs) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse(
				"Verify Process Integrity in Threadify",
				mcp.NewPromptMessage(
					mcp.NewTextContent(`You are verifying the integrity of a business process execution using Threadify's cryptographic hash chain.

**What is Threadify?**
Threadify tracks distributed workflow execution with cryptographic verification. Each step is linked via hash chains (prevHash → hash), making tampering, reordering, or missing steps detectable. This is critical for compliance, audit trails, and ensuring business processes executed correctly.

**Why This Matters:**
Your dashboards may show green (all services up), but business processes can still fail silently:
- Steps executed out of order (e.g., fulfillment before payment)
- Required compliance steps skipped
- Data tampered with after execution
- Race conditions causing invalid state transitions

**Verification Checks:**
1. **Hash chain integrity**: Each step's prevHash must match previous step's hash (cryptographic proof of order)
2. **All required steps executed**: Compare actual steps against contract's required steps
3. **Valid state transitions**: Steps followed contract's allowed transition rules
4. **No validation violations**: All business rules passed (no critical violations)
5. **Correct actors**: Steps executed by authorized services/users

**Investigation Workflow:**
1. **Get thread execution**: Use 'get_thread' to retrieve all steps with their hashes
2. **Verify hash chain**: Use 'verify_thread_integrity' for cryptographic validation
3. **Get expected flow**: Use 'contract_graph' to see required steps and valid transitions
4. **Compare actual vs expected**: Identify missing steps or invalid transitions
5. **Check validation results**: Use 'graphql_query' to get detailed validation violations
6. **Verify actors**: Ensure steps were executed by authorized services

**What to Report:**
- Hash chain status: verified (intact) or broken (tampered/missing steps)
- Missing required steps: list any steps that should have run but didn't
- Out-of-order execution: steps that ran in invalid sequence
- Validation violations: critical business rule failures
- Actor verification: unauthorized step executions
- Overall integrity assessment: pass/fail with specific issues

**Example Use Cases:**
- Compliance audit: prove payment → fulfillment → notification sequence
- Fraud investigation: detect if steps were tampered with post-execution
- Process debugging: find where required steps were skipped
- SLA verification: ensure all contractual steps completed

Start by asking the user for the thread ID to verify.`),
					mcp.RoleUser,
				),
			), nil
		},
	)
	if err != nil {
		logger.Fatal("failed to register verify_process_integrity prompt", zap.Error(err))
	}

	// Prompt 3: Analyze workflow execution
	err = mcpSrv.RegisterPrompt(
		"analyze_workflow",
		"Analyze execution patterns across multiple threads of a workflow/contract to identify bottlenecks, common failures, or performance issues.",
		func(args EmptyArgs) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse(
				"Analyze Workflow Execution in Threadify",
				mcp.NewPromptMessage(
					mcp.NewTextContent(`You are analyzing workflow execution patterns using Threadify to identify operational issues and optimization opportunities.

**What is Threadify?**
Threadify tracks business process execution across distributed systems, providing visibility into which steps run, fail, or get skipped. Unlike infrastructure monitoring (Grafana, Datadog), Threadify monitors business processes - the actual customer-facing workflows.

**Analysis Goals:**
- Identify common failure points in workflows
- Find bottlenecks (steps that take longest or fail most)
- Detect patterns in silent failures (steps that never run)
- Compare successful vs failed executions
- Measure workflow reliability and completion rates
- Identify retry patterns and recovery success

**Investigation Workflow:**
1. **Understand the workflow**: Use 'contract_graph' to see expected steps and transitions
2. **Gather sample threads**: Use 'search_threads' to find recent executions
   - Filter by status (failed, completed, running)
   - Filter by time range (last hour, last day)
   - Get sufficient sample size (10-50 threads)
3. **Analyze each thread**: Use 'get_thread' to see step-by-step execution
4. **Deep dive on failures**: Use 'graphql_query' to get:
   - Step history with timing and error messages
   - Validation results for failed steps
   - Actor information (which services failed)
5. **Aggregate patterns**: Calculate metrics across all threads

**Key Metrics to Calculate:**
- **Success rate per step**: (successful executions / total attempts) × 100
- **Failure rate by step**: Which steps fail most often
- **Average duration per step**: Identify slow steps
- **Completion rate**: Threads that reach terminal steps vs get stuck
- **Retry patterns**: Steps with high retry counts
- **Silent failure rate**: Expected steps that never recorded
- **Validation violation frequency**: Most common business rule failures

**Patterns to Look For:**
- Steps that consistently fail together (correlated failures)
- Time-based patterns (failures during peak hours)
- Actor-based patterns (specific services causing failures)
- Sequence patterns (failures after specific step combinations)
- Missing step patterns (steps that should run but don't)

**What to Report:**
1. **Overall workflow health**: Success rate, completion rate, average duration
2. **Bottleneck identification**: Top 3 slowest or most failure-prone steps
3. **Common failure patterns**: Most frequent error messages and failure sequences
4. **Silent failures**: Steps that are expected but frequently missing
5. **Actionable recommendations**: 
   - Steps that need better error handling
   - Timeout adjustments needed
   - Retry logic improvements
   - Services that need investigation

**Example Analysis:**
"Analyzed 50 order_processing threads from last 24 hours:
- Overall success rate: 82% (41/50 completed)
- Bottleneck: charge_payment step fails 15% of the time (timeout errors)
- Silent failure: send_confirmation step missing in 8 threads despite payment success
- Recommendation: Increase payment gateway timeout, add retry logic for notification step"

Start by asking the user for the contract name and time range to analyze.`),
					mcp.RoleUser,
				),
			), nil
		},
	)
	if err != nil {
		logger.Fatal("failed to register analyze_workflow prompt", zap.Error(err))
	}

	// Connect the MCP server synchronously before returning — this wires the
	// protocol message handlers to the transport so the first request is handled correctly.
	if err := mcpSrv.Serve(); err != nil {
		logger.Fatal("failed to start MCP server", zap.Error(err))
	}

	r.POST("", pullContextAuthMiddleware(), mcpTransport.Handler())
	logger.Info("MCP server mounted", zap.String("path", "/mcp"), zap.Int("tools", 6), zap.Int("resources", 1), zap.Int("prompts", 3))
}

package app

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
	"github.com/threadify/engine/internal/domain"
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
func graphQLQuery(ctx context.Context, apiPort int, cfg *config.Config, planSvc domain.PlanService, logger *zap.Logger, query string, variables map[string]interface{}) (*mcp.ToolResponse, error) {
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

func mountMCPServer(r *gin.RouterGroup, cfg *config.Config, planSvc domain.PlanService, logger *zap.Logger) {
	mcpTransport := mcptransport.NewGinTransport()
	mcpSrv := mcp.NewServer(
		mcpTransport,
		mcp.WithName("threadify"),
		mcp.WithVersion("1.0.0"),
	)
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

	// Register prompts omitted for brevity in this initial port if needed,
	// but I will include them for completeness as they were in the original.

	// Prompt 1: Debug silent failure
	mcpSrv.RegisterPrompt(
		"debug_silent_failure",
		"Investigate why a business process failed silently (service returned 200 but downstream steps never ran).",
		func(args struct{}) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse(
				"Debug Silent Failure in Threadify",
				mcp.NewPromptMessage(
					mcp.NewTextContent(`You are debugging a silent failure in a distributed business process using Threadify.`),
					mcp.RoleUser,
				),
			), nil
		},
	)

	// Connect the MCP server synchronously
	if err := mcpSrv.Serve(); err != nil {
		logger.Fatal("failed to start MCP server", zap.Error(err))
	}

	r.POST("", pullContextAuthMiddleware(), mcpTransport.Handler())
	logger.Info("MCP server mounted", zap.String("path", "/mcp"), zap.Int("tools", 6), zap.Int("resources", 1), zap.Int("prompts", 1))
}

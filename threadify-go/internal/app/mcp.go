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

	// 1. get_entity_profile
	type GetEntityProfileArgs struct {
		RefKey string `json:"refKey" jsonschema_description:"REQUIRED: The customer/partner identifier (e.g., 'CUST-4981', 'user_123'). Use this alongside 'type'."`
		Type   string `json:"type" jsonschema_description:"REQUIRED: The profile type (e.g., 'Customer profile', 'Partner'). Use this alongside 'refKey'."`
		Range  string `json:"range,omitempty" jsonschema_description:"Optional. Time range for computed metrics. Allowed: '7d', '30d', '90d'. Defaults to '7d'."`
	}
	mustRegister("get_entity_profile", mcpSrv.RegisterTool("get_entity_profile", "Get full entity profile including computed metrics, health score, and delivery intelligence", func(ctx context.Context, args GetEntityProfileArgs) (*mcp.ToolResponse, error) {
		vars := map[string]interface{}{}
		if args.RefKey != "" {
			vars["refKey"] = args.RefKey
		}
		if args.Type != "" {
			vars["type"] = args.Type
		}
		if args.Range != "" {
			vars["range"] = args.Range
		}
		return graphQLQuery(ctx, port, cfg, planSvc, logger,
			`query ($refKey: String, $type: String, $range: String) {
				entityProfile(refKey: $refKey, type: $type) {
					id refKey name createdAt lastActiveAt
					metrics { totalDeliveries completedSuccessfully validationViolations deliveryHealthScore prevDeliveryHealthScore healthTrendSlope averageDeliveryTimeMs }
					computedMetrics(range: $range)
				}
			}`,
			vars,
		)
	}), logger)

	// 2. search_threads
	type SearchThreadsArgs struct {
		ContractName    string   `json:"contractName,omitempty" jsonschema_description:"Filter by contract name"`
		Status          string   `json:"status,omitempty" jsonschema_description:"Filter by status: ACTIVE, COMPLETED, FAILED"`
		Actor           string   `json:"actor,omitempty" jsonschema_description:"Filter by actor"`
		Tags            []string `json:"tags,omitempty" jsonschema_description:"Filter by tags"`
		StartedAfter    string   `json:"startedAfter,omitempty" jsonschema_description:"ISO8601 timestamp"`
		CompletedBefore string   `json:"completedBefore,omitempty" jsonschema_description:"ISO8601 timestamp"`
		Limit           int      `json:"limit,omitempty" jsonschema_description:"Maximum number of results (default 50, max 100)"`
	}
	mustRegister("search_threads", mcpSrv.RegisterTool("search_threads", "Search threads with flexible filters (contract, actor, date range, tags, outcome)", func(ctx context.Context, args SearchThreadsArgs) (*mcp.ToolResponse, error) {
		vars := map[string]interface{}{}
		if args.ContractName != "" {
			vars["contractName"] = args.ContractName
		}
		if args.Status != "" {
			vars["status"] = args.Status
		}
		if args.Actor != "" {
			vars["actor"] = args.Actor
		}
		if len(args.Tags) > 0 {
			vars["tags"] = args.Tags
		}
		if args.StartedAfter != "" {
			vars["startedAfter"] = args.StartedAfter
		}
		if args.CompletedBefore != "" {
			vars["completedBefore"] = args.CompletedBefore
		}
		if args.Limit > 0 {
			if args.Limit > 100 {
				vars["limit"] = 100
			} else {
				vars["limit"] = args.Limit
			}
		} else {
			vars["limit"] = 50
		}
		return graphQLQuery(ctx, port, cfg, planSvc, logger,
			`query ($contractName: String, $status: String, $actor: String, $tags: [String!], $startedAfter: String, $completedBefore: String, $limit: Int) {
				threads(contractName: $contractName, status: $status, actor: $actor, tags: $tags, startedAfter: $startedAfter, completedBefore: $completedBefore, limit: $limit) {
					threads { id status contractName contractVersion startedAt completedAt error }
					totalCount
				}
			}`,
			vars,
		)
	}), logger)

	// 3. get_thread
	type GetThreadArgs struct {
		ID string `json:"id" jsonschema_description:"The UUID of the thread"`
	}
	mustRegister("get_thread", mcpSrv.RegisterTool("get_thread", "Deep dive into a single thread. Fetches steps, history, outcomes, actors, and contract validations.", func(ctx context.Context, args GetThreadArgs) (*mcp.ToolResponse, error) {
		return graphQLQuery(ctx, port, cfg, planSvc, logger,
			`query ($id: ID!) { 
				thread(id: $id) { 
					id contractId contractName status startedAt completedAt error refs
					steps { 
						stepName status retryCount actor actorService latestContext 
						history { status timestamp context error duration actor actorService }
					}
					notifications {
						notificationId source notificationType stepStatus validationStatus violationType severity message timestamp
					}
				} 
			}`,
			map[string]interface{}{"id": args.ID},
		)
	}), logger)

	// 4. get_contract_violations
	type GetContractViolationsArgs struct {
		ContractName  string   `json:"contractName,omitempty" jsonschema_description:"Filter by contract name"`
		RefKey        string   `json:"refKey,omitempty" jsonschema_description:"Filter by a specific reference key (e.g., 'customer_id')"`
		RefValue      string   `json:"refValue,omitempty" jsonschema_description:"Filter by a specific reference value"`
		Severity      []string `json:"severity,omitempty" jsonschema_description:"Filter by severity: 'critical', 'warning', 'info'"`
		StartedAfter  string   `json:"startedAfter,omitempty" jsonschema_description:"ISO8601 timestamp"`
		StartedBefore string   `json:"startedBefore,omitempty" jsonschema_description:"ISO8601 timestamp"`
		Limit         int      `json:"limit,omitempty" jsonschema_description:"Maximum number of results (default 50, max 100)"`
	}
	mustRegister("get_contract_violations", mcpSrv.RegisterTool("get_contract_violations", "Retrieve contract violations across multiple threads filterable by contract, entity, and severity", func(ctx context.Context, args GetContractViolationsArgs) (*mcp.ToolResponse, error) {
		vars := map[string]interface{}{}
		if args.ContractName != "" {
			vars["contractName"] = args.ContractName
		}
		if args.RefKey != "" {
			vars["refKey"] = args.RefKey
		}
		if args.RefValue != "" {
			vars["refValue"] = args.RefValue
		}
		if len(args.Severity) > 0 {
			vars["severity"] = args.Severity
		}
		if args.StartedAfter != "" {
			vars["startedAfter"] = args.StartedAfter
		}
		if args.StartedBefore != "" {
			vars["startedBefore"] = args.StartedBefore
		}
		if args.Limit > 0 {
			if args.Limit > 100 {
				vars["limit"] = 100
			} else {
				vars["limit"] = args.Limit
			}
		} else {
			vars["limit"] = 50
		}
		return graphQLQuery(ctx, port, cfg, planSvc, logger,
			`query ($contractName: String, $refKey: String, $refValue: String, $severity: [String!], $startedAfter: String, $startedBefore: String, $limit: Int) {
				contractViolations(contractName: $contractName, refKey: $refKey, refValue: $refValue, severity: $severity, startedAfter: $startedAfter, startedBefore: $startedBefore, limit: $limit) {
					threadId stepName source notificationType severity message details timestamp
				}
			}`,
			vars,
		)
	}), logger)

	// 5. query (formerly graphql_query)
	type GraphQLQueryArgs struct {
		Query     string                 `json:"query" jsonschema_description:"GraphQL query string (required). Use the 'graphql_schema' resource to discover available types and fields."`
		Variables map[string]interface{} `json:"variables,omitempty" jsonschema_description:"Optional query variables as a JSON object"`
	}
	mustRegister("query", mcpSrv.RegisterTool(
		"query",
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

	// Register prompts
	mcpSrv.RegisterPrompt(
		"analyse_entity_delivery",
		"Synthesize a narrative summary of delivery health for a specific entity.",
		func(args struct{}) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse(
				"Analyse Entity Delivery",
				mcp.NewPromptMessage(
					mcp.NewTextContent(`Use the 'get_entity_profile' tool to fetch the entity's profile. Then, synthesize a narrative summary of their delivery health, pointing out top errors, success rates, and any notable trends.`),
					mcp.RoleUser,
				),
			), nil
		},
	)

	mcpSrv.RegisterPrompt(
		"investigate_thread",
		"Deep dive into a single thread to explain what happened and why it failed.",
		func(args struct{}) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse(
				"Investigate Thread",
				mcp.NewPromptMessage(
					mcp.NewTextContent(`Use the 'get_thread' tool to fetch the thread's execution graph. Explain what happened, which step failed, which actors were involved, and the root cause of any contract violations in plain language.`),
					mcp.RoleUser,
				),
			), nil
		},
	)

	mcpSrv.RegisterPrompt(
		"identify_delivery_failures",
		"Surface patterns across failed threads for an entity or contract.",
		func(args struct{}) (*mcp.PromptResponse, error) {
			return mcp.NewPromptResponse(
				"Identify Delivery Failures",
				mcp.NewPromptMessage(
					mcp.NewTextContent(`Use the 'search_threads' tool to filter for 'FAILED' threads for the given entity or contract. Identify any common patterns, failing steps, or actors that appear across these failures.`),
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
	logger.Info("MCP server mounted", zap.String("path", "/mcp"), zap.Int("tools", 5), zap.Int("resources", 1), zap.Int("prompts", 3))
}

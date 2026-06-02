package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

type contextKeyAPIKey struct{}

func pullContextAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		ctx := context.WithValue(c.Request.Context(), contextKeyAPIKey{}, apiKey)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// mcpInitializePatchMiddleware patches incoming JSON-RPC initialize requests
// that lack required params, making the server compatible with clients that
// send minimal or empty initialize payloads.
func mcpInitializePatchMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Next()
			return
		}
		c.Request.Body.Close()

		var msg map[string]json.RawMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			// Not JSON-RPC; pass through unchanged.
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			c.Next()
			return
		}

		method, _ := msg["method"]
		if string(method) == `"initialize"` {
			params, hasParams := msg["params"]
			if !hasParams || string(params) == "null" || string(params) == "{}" {
				msg["params"] = json.RawMessage(`{
					"protocolVersion": "2024-11-05",
					"capabilities": {},
					"clientInfo": {"name":"client","version":"1.0.0"}
				}`)
				patched, _ := json.Marshal(msg)
				body = patched
			}
		}

		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Next()
	}
}

func graphQLQuery(ctx context.Context, apiPort int, query string, variables map[string]interface{}) (*mcp.CallToolResult, any, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx, "POST",
		fmt.Sprintf("http://localhost:%d/graphql", apiPort),
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", "application/json")

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
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("GraphQL error (status %d): %s", resp.StatusCode, string(body))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(body)},
		},
	}, nil, nil
}

func mountMCPServer(r *gin.RouterGroup, cfg *config.Config, planSvc domain.PlanService, logger *zap.Logger) {
	mcpSrv := mcp.NewServer(
		&mcp.Implementation{Name: "threadify", Version: "1.0.0"},
		nil,
	)
	port := cfg.Server.Port

	type GetEntityProfileArgs struct {
		RefKey string `json:"refKey" jsonschema_description:"REQUIRED: The customer/partner identifier (e.g., 'CUST-4981', 'user_123'). Use this alongside 'type'."`
		Type   string `json:"type" jsonschema_description:"REQUIRED: The profile type (e.g., 'Customer profile', 'Partner'). Use this alongside 'refKey'."`
		Range  string `json:"range,omitempty" jsonschema_description:"Optional. Time range for computed metrics. Allowed: '7d', '30d', '90d'. Defaults to '7d'."`
	}
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "get_entity_profile",
		Description: "Get full entity profile including computed metrics, health score, and delivery intelligence",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetEntityProfileArgs) (*mcp.CallToolResult, any, error) {
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
		return graphQLQuery(ctx, port,
			`query ($refKey: String, $type: String, $range: String) {
				entityProfile(refKey: $refKey, type: $type) {
					id refKey name createdAt lastActiveAt
					metrics { totalDeliveries completedSuccessfully validationViolations deliveryHealthScore prevDeliveryHealthScore healthTrendSlope averageDeliveryTimeMs }
					computedMetrics(range: $range)
				}
			}`,
			vars,
		)
	})

	type SearchThreadsArgs struct {
		ContractName    string   `json:"contractName,omitempty" jsonschema_description:"Filter by contract name"`
		Status          string   `json:"status,omitempty" jsonschema_description:"Filter by status: ACTIVE, COMPLETED, FAILED"`
		Actor           string   `json:"actor,omitempty" jsonschema_description:"Filter by actor"`
		Tags            []string `json:"tags,omitempty" jsonschema_description:"Filter by tags"`
		StartedAfter    string   `json:"startedAfter,omitempty" jsonschema_description:"ISO8601 timestamp"`
		CompletedBefore string   `json:"completedBefore,omitempty" jsonschema_description:"ISO8601 timestamp"`
		Limit           int      `json:"limit,omitempty" jsonschema_description:"Maximum number of results (default 50, max 100)"`
	}
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "search_threads",
		Description: "Search threads with flexible filters (contract, actor, date range, tags, outcome)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SearchThreadsArgs) (*mcp.CallToolResult, any, error) {
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
		return graphQLQuery(ctx, port,
			`query ($contractName: String, $status: String, $actor: String, $tags: [String!], $startedAfter: String, $completedBefore: String, $limit: Int) {
				threads(contractName: $contractName, status: $status, actor: $actor, tags: $tags, startedAfter: $startedAfter, completedBefore: $completedBefore, limit: $limit) {
					threads { id status contractName contractVersion startedAt completedAt error }
					totalCount
				}
			}`,
			vars,
		)
	})

	type GetThreadArgs struct {
		ID string `json:"id" jsonschema_description:"The UUID of the thread"`
	}
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "get_thread",
		Description: "Deep dive into a single thread. Fetches steps, history, outcomes, actors, and contract validations.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetThreadArgs) (*mcp.CallToolResult, any, error) {
		return graphQLQuery(ctx, port,
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
	})

	type GetContractViolationsArgs struct {
		ContractName  string   `json:"contractName,omitempty" jsonschema_description:"Filter by contract name"`
		RefKey        string   `json:"refKey,omitempty" jsonschema_description:"Filter by a specific reference key (e.g., 'customer_id')"`
		RefValue      string   `json:"refValue,omitempty" jsonschema_description:"Filter by a specific reference value"`
		Severity      []string `json:"severity,omitempty" jsonschema_description:"Filter by severity: 'critical', 'warning', 'info'"`
		StartedAfter  string   `json:"startedAfter,omitempty" jsonschema_description:"ISO8601 timestamp"`
		StartedBefore string   `json:"startedBefore,omitempty" jsonschema_description:"ISO8601 timestamp"`
		Limit         int      `json:"limit,omitempty" jsonschema_description:"Maximum number of results (default 50, max 100)"`
	}
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "get_contract_violations",
		Description: "Retrieve contract violations across multiple threads filterable by contract, entity, and severity",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetContractViolationsArgs) (*mcp.CallToolResult, any, error) {
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
		return graphQLQuery(ctx, port,
			`query ($contractName: String, $refKey: String, $refValue: String, $severity: [String!], $startedAfter: String, $startedBefore: String, $limit: Int) {
				contractViolations(contractName: $contractName, refKey: $refKey, refValue: $refValue, severity: $severity, startedAfter: $startedAfter, startedBefore: $startedBefore, limit: $limit) {
					threadId stepName source notificationType severity message details timestamp
				}
			}`,
			vars,
		)
	})

	type GraphQLQueryArgs struct {
		Query     string                 `json:"query" jsonschema_description:"GraphQL query string (required). Use the 'graphql_schema' resource to discover available types and fields."`
		Variables map[string]interface{} `json:"variables,omitempty" jsonschema_description:"Optional query variables as a JSON object"`
	}
	mcp.AddTool(mcpSrv, &mcp.Tool{
		Name:        "query",
		Description: "Execute custom GraphQL queries for precise data retrieval. Use when specialized tools don't provide exact fields needed. Check 'graphql_schema' resource for available fields.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GraphQLQueryArgs) (*mcp.CallToolResult, any, error) {
		if args.Variables == nil {
			args.Variables = map[string]interface{}{}
		}
		return graphQLQuery(ctx, port, args.Query, args.Variables)
	})

	mcpSrv.AddResource(&mcp.Resource{
		URI:         "graphql://schema",
		Name:        "graphql_schema",
		Description: "Threadify GraphQL schema - shows all available queries for investigating business process execution. Threadify tracks distributed workflow execution step-by-step, helping debug silent failures where services succeed but downstream processes never run. Use this schema to discover how to query threads, steps, validation results, and cryptographic integrity verification.",
		MIMEType:    "application/graphql",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		content, err := os.ReadFile("internal/graphql/schema.graphql")
		if err != nil {
			return nil, fmt.Errorf("failed to read schema: %w", err)
		}
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{
					URI:      "graphql://schema",
					MIMEType: "application/graphql",
					Text:     string(content),
				},
			},
		}, nil
	})

	mcpSrv.AddPrompt(&mcp.Prompt{
		Name:        "analyse_entity_delivery",
		Description: "Synthesize a narrative summary of delivery health for a specific entity.",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Analyse Entity Delivery",
			Messages: []*mcp.PromptMessage{
				{
					Role:    "user",
					Content: &mcp.TextContent{Text: "Use the 'get_entity_profile' tool to fetch the entity's profile. Then, synthesize a narrative summary of their delivery health, pointing out top errors, success rates, and any notable trends."},
				},
			},
		}, nil
	})

	mcpSrv.AddPrompt(&mcp.Prompt{
		Name:        "investigate_thread",
		Description: "Deep dive into a single thread to explain what happened and why it failed.",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Investigate Thread",
			Messages: []*mcp.PromptMessage{
				{
					Role:    "user",
					Content: &mcp.TextContent{Text: "Use the 'get_thread' tool to fetch the thread's execution graph. Explain what happened, which step failed, which actors were involved, and the root cause of any contract violations in plain language."},
				},
			},
		}, nil
	})

	mcpSrv.AddPrompt(&mcp.Prompt{
		Name:        "identify_delivery_failures",
		Description: "Surface patterns across failed threads for an entity or contract.",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Identify Delivery Failures",
			Messages: []*mcp.PromptMessage{
				{
					Role:    "user",
					Content: &mcp.TextContent{Text: "Use the 'search_threads' tool to filter for 'FAILED' threads for the given entity or contract. Identify any common patterns, failing steps, or actors that appear across these failures."},
				},
			},
		}, nil
	})

	streamHandler := mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server {
		return mcpSrv
	}, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		DisableLocalhostProtection: false,
	})

	// Patch Accept header for clients that don't send both required types.
	acceptPatchMiddleware := func(c *gin.Context) {
		if c.Request.Method == http.MethodPost || c.Request.Method == http.MethodGet {
			accept := c.Request.Header.Get("Accept")
			if accept != "" && accept != "*/*" {
				// Ensure both JSON and SSE are accepted.
				if !strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/event-stream") {
					c.Request.Header.Set("Accept", accept+", application/json, text/event-stream")
				}
			}
		}
		c.Next()
	}

	r.Any("", acceptPatchMiddleware, pullContextAuthMiddleware(), mcpInitializePatchMiddleware(), gin.WrapH(streamHandler))
	r.Any("/", acceptPatchMiddleware, pullContextAuthMiddleware(), mcpInitializePatchMiddleware(), gin.WrapH(streamHandler))

	logger.Info("MCP server mounted", zap.String("path", "/sse"))
}

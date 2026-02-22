package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

type ChatRequest struct {
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id"`
}

type AgentHandler struct {
	threadifyEngineURL string
	httpClient         *http.Client
	openaiClient       *openai.Client
	agentRepo          *repository.AgentRepository
}

func NewAgentHandler(threadifyEngineURL string, apiKey string, agentRepo *repository.AgentRepository) *AgentHandler {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = "https://api.deepseek.com/v1"
	client := openai.NewClientWithConfig(config)

	return &AgentHandler{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{},
		openaiClient:       client,
		agentRepo:          agentRepo,
	}
}

// executeGraphQL calls the internal engine directly
func (h *AgentHandler) executeGraphQL(authHeader, query string, variables map[string]interface{}) (string, error) {
	reqBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", h.threadifyEngineURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Threadify-AI-Agent/1.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(respBody), nil
}

// Chat handles natural language queries related to thread analysis
func (h *AgentHandler) Chat(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return
	}

	if h.openaiClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI service is not configured"})
		return
	}

	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	companyID, exists := c.Get("companyID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var chatReq ChatRequest
	if err := c.ShouldBindJSON(&chatReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body format"})
		return
	}

	convID := chatReq.ConversationID
	isNewConversation := false

	if convID == "" {
		convID = uuid.New().String()
		isNewConversation = true
		title := chatReq.Message
		if len(title) > 30 {
			title = title[:30] + "..."
		}
		err := h.agentRepo.CreateConversation(&models.AgentConversation{
			ID:        convID,
			UserID:    userID.(string),
			CompanyID: companyID.(string),
			Title:     title,
		})
		if err != nil {
			log.Printf("Failed to create conversation: %v", err)
		}
	} else {
		// verify existence + permission by loading messages
		// we should actually check if companyID matches
	}

	// Save the user's message
	err := h.agentRepo.AddMessage(&models.AgentMessage{
		ID:             uuid.New().String(),
		ConversationID: convID,
		Role:           "user",
		Content:        chatReq.Message,
	})
	if err != nil {
		log.Printf("Failed to save user message: %v", err)
	}

	// 1. Initial State (LangGraph pattern) -> Loading History vs Fresh
	systemPrompt := openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleSystem,
		Content: `You are Threadify's systems analyst AI. Be EXTREMELY CONCISE.

CRITICAL RULES:
1. **Brevity First** - Give SHORT, direct answers. No fluff.
2. **Minimal GraphQL** - ONLY fetch fields needed. Examples:
   - Need thread count? Query: threads(limit: 1) { totalCount }
   - Need thread IDs? Query: threads { id }
   - Need step names? Query: thread(id: "x") { steps { stepName } }
   - DON'T fetch: full step history, context, or nested data unless explicitly asked
3. **No Pre-loaded Data** - Always use 'execute_graphql' tool first
4. **One Query** - Combine filters instead of multiple queries when possible

Available GraphQL Queries outline (use this schema for reference):

Type: Query {
  # Get threads with filtering (company-scoped automatically via JWT)
  threads(
    actor: String, contractName: String, contractVersion: Int, status: String,
    startedAfter: String, startedBefore: String, limit: Int = 50, offset: Int = 0
  ): ThreadConnection!
  
  # Get a single thread by ID
  thread(id: ID!): Thread
}

Type: ThreadConnection {
  threads: [Thread!]!
  totalCount: Int!
}

Type: Thread {
  id: ID!
  contractName: String
  contractVersion: Int
  status: String!
  startedAt: String!
  completedAt: String
  steps: [StepStateInfo!]!
}

Type: StepStateInfo {
  stepName: String!
  idempotencyKey: String!
  status: String!
  retryCount: Int
  latestStepID: String
  firstSeenAt: String
  lastUpdatedAt: String
  actor: String
  actorService: String
}

IMPORTANT QUERY EXAMPLES:
- Get thread count: threads(limit: 1) { totalCount }
- Get recent threads: threads(limit: 10) { threads { id contractName status } }
- Get specific thread: thread(id: "abc") { id status steps { stepName status } }
- Filter by contract: threads(contractName: "order-processing") { threads { id status } }
- Filter by status: threads(status: "failed") { threads { id contractName } }
- Get step details: thread(id: "abc") { steps { stepName status retryCount actor } }

Remember: ONLY fetch fields you need. Don't fetch error fields - they don't exist on steps.

Core Concepts regarding Threadify you must know:
1. Thread - One customer request flowing through the system
2. Step - One action in the workflow
3. Contract - YAML validation rules enforced at runtime
4. Step Statuses:
   - success: Step completed as expected (e.g., Payment processed)
   - failed: Business logic failure (e.g., Payment declined)
   - error: System/technical error (e.g., Payment gateway timeout)
5. Idempotency prevents duplicate step execution.
6. Context values are flat string key-value pairs representing business input/outputs for each step execution history.

Follow this exact process when responding:
1. Identify the user's intent. Do they want to find generic failed threads? Search by contract? Or analyze a specific thread ID?
2. Call 'execute_graphql'. You MUST provide precise syntactically valid GraphQL strings.
3. Review the returned JSON result.
4. Synthesize the final answer using the fetched raw data.

Never refuse to try to look up data unless the user's query is completely unrelated to technical systems/execution tracing. Give concrete details based on the payloads.`,
	}

	messages := []openai.ChatCompletionMessage{systemPrompt}

	if !isNewConversation {
		history, err := h.agentRepo.GetMessages(convID)
		if err == nil {
			for _, m := range history {
				msg := openai.ChatCompletionMessage{
					Role:    m.Role,
					Content: m.Content,
				}
				if m.ToolCallID != nil && *m.ToolCallID != "" {
					msg.ToolCallID = *m.ToolCallID
				}
				if m.ToolCalls != nil && *m.ToolCalls != "" {
					var rawCalls []openai.ToolCall
					if err := json.Unmarshal([]byte(*m.ToolCalls), &rawCalls); err == nil {
						msg.ToolCalls = rawCalls
					}
				}
				messages = append(messages, msg)
			}
		}
	} else {
		// New conversation, just append current prompt
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: chatReq.Message,
		})
	}

	// Define the GraphQL execution tool based on LangGraph Tool Calling specification
	tools := []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "execute_graphql",
				Description: "Execute a GraphQL query against the Threadify engine to retrieve thread data, execution history, and contract state. You can pass 'query' as a string and 'variables' as a JSON map.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"query": { "type": "string", "description": "The GraphQL query to execute." },
						"variables": { "type": "object", "description": "JSON object of variables for the query." }
					},
					"required": ["query"]
				}`),
			},
		},
	}

	// SSE Header setup
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	// 2. The loop (LangGraph edge condition loop)
	for i := 0; i < 5; i++ { // limit recursion depth
		req := openai.ChatCompletionRequest{
			Model:    "deepseek-chat",
			Messages: messages,
			Tools:    tools,
			Stream:   true,
		}

		stream, err := h.openaiClient.CreateChatCompletionStream(context.Background(), req)
		if err != nil {
			log.Printf("[AGENT ERROR] %v", err)
			return
		}

		var currentContent, currentToolId, currentToolName, currentToolArgs string
		var hasToolCalls bool

		// Stream Processor
		for {
			response, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("[STREAM ERROR] %v", err)
				return
			}

			if len(response.Choices) == 0 {
				continue
			}

			delta := response.Choices[0].Delta

			// Stream text back to the client natively!
			if delta.Content != "" {
				currentContent += delta.Content
				c.SSEvent("chunk", delta.Content)
				c.Writer.Flush()
			}

			// Accumulate Tool Calls
			if len(delta.ToolCalls) > 0 {
				hasToolCalls = true
				tc := delta.ToolCalls[0]
				if tc.ID != "" {
					currentToolId = tc.ID
				}
				if tc.Function.Name != "" {
					currentToolName = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					currentToolArgs += tc.Function.Arguments
				}
			}
		}

		stream.Close()

		if hasToolCalls {
			// Add tool call to messages for next LLM iteration
			assistantToolMsg := openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{
					{
						ID:   currentToolId,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      currentToolName,
							Arguments: currentToolArgs,
						},
					},
				},
			}
			messages = append(messages, assistantToolMsg)

			// Inform user UI that data is being loaded behind the scenes
			c.SSEvent("system", "Querying Threadify Engine...")
			c.Writer.Flush()

			// Tool node layer executes logic
			if currentToolName == "execute_graphql" {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(currentToolArgs), &args); err != nil {
					log.Printf("Tool args parsing error: %v", err)
					break
				}

				query, _ := args["query"].(string)
				variables, _ := args["variables"].(map[string]interface{})

				log.Printf("[AGENT] Executing GraphQL tool call for query: %s", query)

				// Use original proxy
				engineOutput, err := h.executeGraphQL(authHeader, query, variables)
				if err != nil {
					engineOutput = fmt.Sprintf("{\"error\": \"%v\"}", err)
				}

				// Add tool result to messages
				toolResultMsg := openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    engineOutput,
					ToolCallID: currentToolId,
				}
				messages = append(messages, toolResultMsg)

				// Save both messages to DB (assistant tool call + tool result)
				toolCallJSON, _ := json.Marshal([]openai.ToolCall{
					{
						ID:   currentToolId,
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      currentToolName,
							Arguments: currentToolArgs,
						},
					},
				})
				tcStr := string(toolCallJSON)

				// Save AI tool invocation
				_ = h.agentRepo.AddMessage(&models.AgentMessage{
					ID:             uuid.New().String(),
					ConversationID: convID,
					Role:           "assistant",
					Content:        "",
					ToolCalls:      &tcStr,
				})

				// Save Tool Execution Result
				_ = h.agentRepo.AddMessage(&models.AgentMessage{
					ID:             uuid.New().String(),
					ConversationID: convID,
					Role:           "tool",
					Content:        engineOutput,
					ToolCallID:     &currentToolId,
				})

				// Loop continues! (jump to next llm call with data)
				continue
			}
		}

		// Save any generated final text
		if currentContent != "" {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: currentContent,
			})

			_ = h.agentRepo.AddMessage(&models.AgentMessage{
				ID:             uuid.New().String(),
				ConversationID: convID,
				Role:           "assistant",
				Content:        currentContent,
			})
		}

		// Send final payload with conversation info
		c.SSEvent("conversation", convID)
		c.SSEvent("done", true)
		c.Writer.Flush()
		return
	}

	c.SSEvent("error", "Agent recursion limits exceeded")
	c.Writer.Flush()
}

// GetConversations lists recent conversations for a user
func (h *AgentHandler) GetConversations(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	convs, err := h.agentRepo.GetConversations(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load conversations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"conversations": convs})
}

// GetConversation returns a specific conversation history
func (h *AgentHandler) GetConversation(c *gin.Context) {
	convID := c.Param("id")
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Quick authorization - ensure user owns the conversation
	convs, err := h.agentRepo.GetConversations(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load conversation history"})
		return
	}

	owns := false
	for _, conv := range convs {
		if conv.ID == convID {
			owns = true
			break
		}
	}

	if !owns {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to view this conversation"})
		return
	}

	msgs, err := h.agentRepo.GetMessages(convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load messages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": msgs})
}

// DeleteConversation deletes a conversation and all its messages
func (h *AgentHandler) DeleteConversation(c *gin.Context) {
	convID := c.Param("id")
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err := h.agentRepo.DeleteConversation(convID, userID.(string))
	if err != nil {
		if err.Error() == "conversation not found or not owned by user" {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete conversation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted successfully"})
}

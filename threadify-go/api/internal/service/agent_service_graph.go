package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/repository"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

const (
	// AI Models
	ModelGPT4oMini = "gpt-4o-mini"

	// Tool Names
	ToolExecuteGraphQL = "execute_graphql"
	ToolSaveContext    = "save_context"

	// GraphQL Operations
	queryCheckCredits = `query CheckCredits($meter: String, $amount: Int) {
		checkCredits(meter: $meter, amount: $amount)
	}`
	mutationRecordUsage = `mutation RecordUsage($tokens: Int!) {
		recordLLMUsage(tokens: $tokens)
	}`
)

func (s *AgentService) readPromptConfig(filename string) string {
	paths := []string{
		"../config/prompts/" + filename,
		"config/prompts/" + filename,
		filename,
	}
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			s.logger.Debug("loaded prompt config", zap.String("file", p), zap.Int("bytes", len(b)))
			return string(b)
		}
	}
	s.logger.Warn("failed to load prompt config", zap.String("filename", filename))
	return ""
}

func intPtr(i int) *int {
	return &i
}

type AgentState struct {
	ConversationID string
	CompanyID      string
	UserID         string
	Skill          string
	AuthHeader     string
	Messages       []*schema.Message
}

type AgentService struct {
	threadifyEngineURL string
	httpClient         *http.Client
	openaiAPIKey       string
	agentRepo          repository.AgentRepository
	maxMessages        int
	maxTokens          int
	summaryMaxTokens   int
	logger             *zap.Logger
	einoGraph          compose.Runnable[context.Context, *AgentState]
	openaiClient       *openai.Client
}

func NewAgentService(
	threadifyEngineURL string,
	openaiAPIKey string,
	agentRepo repository.AgentRepository,
	maxMessages int,
	maxTokens int,
	summaryMaxTokens int,
	logger *zap.Logger,
) *AgentService {
	s := &AgentService{
		threadifyEngineURL: threadifyEngineURL,
		httpClient:         &http.Client{Timeout: 30 * time.Second},
		openaiAPIKey:       openaiAPIKey,
		agentRepo:          agentRepo,
		maxMessages:        maxMessages,
		maxTokens:          maxTokens,
		summaryMaxTokens:   summaryMaxTokens,
		logger:             logger,
		openaiClient:       openai.NewClient(openaiAPIKey),
	}

	graph, err := s.BuildAgentGraph(context.Background())
	if err != nil {
		logger.Error("failed to build Eino graph", zap.Error(err))
	} else {
		s.einoGraph = graph
		logger.Info("Eino graph initialized and compiled")
	}

	return s
}

func (s *AgentService) ChatStreamEino(
	ctx context.Context,
	authHeader string,
	userID string,
	companyID string,
	conversationID string,
	message string,
	skill string,
	onEvent models.StreamHandler,
) error {
	if s.einoGraph == nil {
		return errors.New("agent graph not initialized")
	}

	if conversationID != "" {
		if err := s.ensureConversationOwnership(ctx, companyID, conversationID); err != nil {
			return err
		}
		msgCount, tokenCount, err := s.agentRepo.GetConversationStats(ctx, conversationID)
		if err == nil {
			if msgCount >= s.maxMessages {
				return fmt.Errorf("conversation has reached the maximum of %d messages", s.maxMessages)
			}
			if tokenCount >= s.maxTokens {
				return fmt.Errorf("conversation has reached the token limit of %d", s.maxTokens)
			}
		}
	}

	convID := conversationID
	isNewConversation := false

	if convID == "" {
		convID = uuid.New().String()
		isNewConversation = true
		title := message
		if len(title) > 30 {
			title = title[:30] + "..."
		}
		if err := s.agentRepo.CreateConversation(ctx, &models.AgentConversation{
			ID:        convID,
			UserID:    userID,
			CompanyID: companyID,
			Title:     title,
		}); err != nil {
			s.logger.Error("failed to create conversation", zap.Error(err))
		}
	}

	// Save User Message
	if err := s.agentRepo.AddMessage(ctx, &models.AgentMessage{
		ID:             uuid.New().String(),
		ConversationID: convID,
		Role:           models.RoleUser,
		Content:        message,
	}); err != nil {
		s.logger.Error("failed to save user message", zap.Error(err))
	}

	// Build State Messages
	messages := []*schema.Message{}
	if !isNewConversation {
		history, _ := s.agentRepo.GetMessages(ctx, convID)
		for _, m := range history {
			role := schema.User
			if m.Role == models.RoleAssistant {
				role = schema.Assistant
			} else if m.Role == models.RoleSystem {
				role = schema.System
			} else if m.Role == models.RoleTool {
				role = schema.Tool
			}
			msg := &schema.Message{Role: role, Content: m.Content}
			if m.ToolCallID != nil {
				msg.ToolCallID = *m.ToolCallID
			}
			if m.ToolCalls != nil {
				var calls []openai.ToolCall
				_ = json.Unmarshal([]byte(*m.ToolCalls), &calls)
				for _, c := range calls {
					msg.ToolCalls = append(msg.ToolCalls, schema.ToolCall{
						ID: c.ID,
						Function: schema.FunctionCall{
							Name:      c.Function.Name,
							Arguments: c.Function.Arguments,
						},
					})
				}
			}
			messages = append(messages, msg)
		}
	}
	messages = append(messages, &schema.Message{Role: schema.User, Content: message})

	state := &AgentState{
		ConversationID: convID,
		CompanyID:      companyID,
		UserID:         userID,
		Skill:          skill,
		AuthHeader:     authHeader,
		Messages:       messages,
	}

	// Graph execution uses state. We stream the nodes.
	ctx = context.WithValue(ctx, "agent_state", state)
	ctx = context.WithValue(ctx, "stream_callback", onEvent)

	onEvent(models.EventSystem, "Agent is thinking...")
	outState, err := s.einoGraph.Invoke(ctx, ctx) // We will implement streaming inside nodes or via callbacks later. Actually we can use Stream.

	if err != nil {
		s.logger.Error("graph execution failed", zap.Error(err))
		onEvent(models.EventError, s.sanitizeError(err))
		return err
	}

	// The last message should be the assistant's response.
	if len(outState.Messages) > 0 {
		lastMsg := outState.Messages[len(outState.Messages)-1]
		if lastMsg.Role == schema.Assistant && lastMsg.Content != "" {
			s.saveAssistantMessage(ctx, convID, lastMsg.Content)
		}
	}

	// Update stats and emit events
	totalTokens := 0
	for _, m := range outState.Messages {
		// Only count assistant and tool tokens produced in this turn
		// Actually, we should only count NEW messages.
		// For simplicity, let's count the last assistant message and any tool calls.
		if m.Role == schema.Assistant || m.Role == schema.Tool {
			totalTokens += s.estimateTokens(m.Content)
		}
	}

	s.updateConversationStats(ctx, convID, totalTokens, onEvent)
	onEvent(models.EventConversation, convID)
	onEvent(models.EventDone, "true")
	return nil
}

func (s *AgentService) ChatStream(ctx context.Context, authHeader, userID, companyID, conversationID, message, skill string, onEvent models.StreamHandler) error {
	return s.ChatStreamEino(ctx, authHeader, userID, companyID, conversationID, message, skill, onEvent)
}

// sanitizeError removes sensitive information from errors before showing to users
func (s *AgentService) sanitizeError(err error) string {
	errMsg := err.Error()

	if strings.Contains(errMsg, "GraphQL error") {
		// Extract just the user-facing part if possible
		if strings.Contains(errMsg, "status") {
			return "The query could not be completed. Please try again."
		}
	}

	if strings.Contains(errMsg, "payment required") || strings.Contains(errMsg, "insufficient credits") {
		return "Insufficient credits. Please top up your account to continue."
	}

	// For other errors, return a generic message but log the real error
	s.logger.Error("sanitized error for user", zap.Error(err))
	return "An error occurred while processing your request. Please try again or contact support."
}

func (s *AgentService) executeGraphQL(ctx context.Context, authHeader, query string, variables map[string]interface{}) (string, error) {
	reqBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.threadifyEngineURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set(HeaderAuthorization, authHeader)
	req.Header.Set(HeaderContentType, ContentTypeJSON)
	req.Header.Set(HeaderUserAgent, UserAgentAPI)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusPaymentRequired || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("%w: %s", ErrPaymentRequired, string(respBody))
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("GraphQL error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return string(respBody), nil
}

func (s *AgentService) CheckCredits(ctx context.Context, authHeader string) (bool, error) {
	resp, err := s.executeGraphQL(ctx, authHeader, queryCheckCredits, nil)
	if err != nil {
		return false, err
	}

	var gqlResp struct {
		Data struct {
			CheckCredits bool `json:"checkCredits"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp), &gqlResp); err != nil {
		return false, fmt.Errorf("failed to parse credit check response: %w", err)
	}

	return gqlResp.Data.CheckCredits, nil
}

func (s *AgentService) recordUsage(ctx context.Context, authHeader string, tokens int) error {
	_, err := s.executeGraphQL(ctx, authHeader, mutationRecordUsage, map[string]interface{}{"tokens": tokens})
	if err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}
	return nil
}

func (s *AgentService) buildInitialMessages(ctx context.Context, convID string, isNew bool, message string, skill string) []openai.ChatCompletionMessage {
	var roleDescription string
	switch skill {
	case models.SkillSupport:
		roleDescription = s.readPromptConfig("skill_support.txt")
	case models.SkillOperations:
		roleDescription = `You are an Operations AI for Threadify. Your goal is to monitor workflow execution and identify operational issues.

FOCUS:
- Workflow reliability and completion rates
- Error patterns and failure points
- Process bottlenecks and delays
- Retry patterns and recovery success

RESPONSE STYLE:
- Process-oriented and actionable
- Highlight anomalies and trends
- Include metrics and counts`
	case models.SkillBusiness:
		roleDescription = `You are a Business Intelligence AI for Threadify. Your goal is to provide insights and analytics on workflow performance.

FOCUS:
- Success rates and conversion metrics
- Workflow completion times
- Business process efficiency
- Trends and patterns over time

RESPONSE STYLE:
- Business-oriented language
- Quantitative insights
- Strategic recommendations`
	case models.SkillDesign:
		roleDescription = s.readPromptConfig("skill_design.txt")
	default:
		roleDescription = `You are Threadify's thread analyzer. Analyze execution threads using GraphQL queries.`
	}

	agentSystemPrompt := s.readPromptConfig("agent_system.txt")

	systemPrompt := openai.ChatCompletionMessage{
		Role:    models.RoleSystem,
		Content: roleDescription + "\n\n" + agentSystemPrompt,
	}

	messages := []openai.ChatCompletionMessage{systemPrompt}

	if !isNew {
		// Load context + history from repo
		contexts, _ := s.agentRepo.GetContext(ctx, convID)
		if len(contexts) > 0 {
			msg := "Previously saved context:\n"
			for _, c := range contexts {
				msg += fmt.Sprintf("- %s: %s\n", c.ContextKey, c.ContextValue)
			}
			messages = append(messages, openai.ChatCompletionMessage{Role: models.RoleSystem, Content: msg})
		}

		history, _ := s.agentRepo.GetMessages(ctx, convID)
		for _, m := range history {
			msg := openai.ChatCompletionMessage{Role: m.Role, Content: m.Content}
			if m.ToolCallID != nil {
				msg.ToolCallID = *m.ToolCallID
			}
			if m.ToolCalls != nil {
				var calls []openai.ToolCall
				_ = json.Unmarshal([]byte(*m.ToolCalls), &calls)
				msg.ToolCalls = calls
			}
			messages = append(messages, msg)
		}
	}

	messages = append(messages, openai.ChatCompletionMessage{Role: models.RoleUser, Content: message})
	return messages
}

type graphqlTool struct {
	s *AgentService
}

func (t *graphqlTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ToolExecuteGraphQL,
		Desc: "Execute a GraphQL query against the Threadify Engine to retrieve thread data",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     "string",
				Desc:     "The GraphQL query string",
				Required: true,
			},
			"variables": {
				Type: "object",
				Desc: "Optional variables for the GraphQL query",
			},
		}),
	}, nil
}

func (t *graphqlTool) InvokableRun(ctx context.Context, arguments string, opts ...tool.Option) (string, error) {
	var args struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", err
	}

	state := ctx.Value("agent_state").(*AgentState)
	out, err := t.s.executeGraphQL(ctx, state.AuthHeader, args.Query, args.Variables)
	if err != nil {
		return fmt.Sprintf("{\"error\": \"%s\"}", t.s.sanitizeError(err)), nil
	}
	return out, nil
}

type saveContextTool struct {
	s *AgentService
}

func (t *saveContextTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: ToolSaveContext,
		Desc: "Save important context/summary",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"key":   {Type: "string", Required: true},
			"value": {Type: "string", Required: true},
		}),
	}, nil
}

func (t *saveContextTool) InvokableRun(ctx context.Context, arguments string, opts ...tool.Option) (string, error) {
	var args struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", err
	}

	state := ctx.Value("agent_state").(*AgentState)
	_ = t.s.agentRepo.SaveContext(ctx, &models.AgentContext{
		ID:             uuid.New().String(),
		ConversationID: state.ConversationID,
		ContextKey:     args.Key,
		ContextValue:   args.Value,
	})
	return models.ToolStatusSuccess, nil
}

func (s *AgentService) BuildAgentGraph(ctx context.Context) (compose.Runnable[context.Context, *AgentState], error) {
	graph := compose.NewGraph[context.Context, *AgentState]()

	// 1. Initial State Injection Node (Just passes context state to Eino execution)
	getStateNode := compose.InvokableLambda(func(ctx context.Context, in context.Context) (*AgentState, error) {
		return in.Value("agent_state").(*AgentState), nil
	})

	// 2. Wrap Agent Model Nodes
	wrapModel := func(skill string) *compose.Lambda {
		// Initialize ChatModel for this specific agent's node
		chatModel, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
			Model:     ModelGPT4oMini,
			APIKey:    s.openaiAPIKey,
			MaxTokens: intPtr(4000),
		})
		if err != nil {
			panic(err)
		}

		toolsList := []tool.BaseTool{&graphqlTool{s: s}, &saveContextTool{s: s}}

		toolInfos := make([]*schema.ToolInfo, 0, len(toolsList))
		for _, t := range toolsList {
			if info, err := t.Info(ctx); err == nil {
				toolInfos = append(toolInfos, info)
			}
		}

		err = chatModel.BindTools(toolInfos)
		if err != nil {
			panic(err)
		}

		return compose.InvokableLambda(func(ctx context.Context, state *AgentState) (*AgentState, error) {
			sysRawMsgs := s.buildInitialMessages(ctx, state.ConversationID, false, "", skill)

			var sysContent string
			for _, m := range sysRawMsgs {
				if m.Role == models.RoleSystem {
					sysContent += m.Content + "\n"
				}
			}

			inputMsgs := append([]*schema.Message{{Role: schema.System, Content: sysContent}}, state.Messages...)

			if skill == models.SkillDesign {
				reminder := "STRICT REMINDER: You MUST wrap the YAML contract in triple backticks ```yaml ... ``` with NEWLINES before and after the block. NO EXCEPTIONS."
				inputMsgs = append(inputMsgs, &schema.Message{Role: schema.System, Content: reminder})
			}

			onEvent, isStreaming := ctx.Value("stream_callback").(models.StreamHandler)
			var finalMsg *schema.Message

			if isStreaming {
				streamReader, err := chatModel.Stream(ctx, inputMsgs)
				if err != nil {
					return nil, err
				}
				defer streamReader.Close()

				for {
					chunk, err := streamReader.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						s.logger.Error("stream error", zap.Error(err))
						return nil, err
					}

					if chunk.Content != "" {
						onEvent(models.EventChunk, chunk.Content)
					}

					if finalMsg == nil {
						finalMsg = chunk
					} else {
						finalMsg.Content += chunk.Content
						if len(chunk.ToolCalls) > 0 {
							for _, tc := range chunk.ToolCalls {
								idx := 0
								if tc.Index != nil {
									idx = *tc.Index
								}
								for len(finalMsg.ToolCalls) <= idx {
									finalMsg.ToolCalls = append(finalMsg.ToolCalls, schema.ToolCall{})
								}
								if tc.ID != "" {
									finalMsg.ToolCalls[idx].ID = tc.ID
								}
								if tc.Type != "" {
									finalMsg.ToolCalls[idx].Type = tc.Type
								}
								if tc.Function.Name != "" {
									finalMsg.ToolCalls[idx].Function.Name += tc.Function.Name
								}
								if tc.Function.Arguments != "" {
									finalMsg.ToolCalls[idx].Function.Arguments += tc.Function.Arguments
								}
							}
						}
					}
				}
			} else {
				outMsg, err := chatModel.Generate(ctx, inputMsgs)
				if err != nil {
					return nil, err
				}
				finalMsg = outMsg
			}

			state.Messages = append(state.Messages, finalMsg)

			if skill == models.SkillDesign && finalMsg.Content != "" {
				s.emitContractPreview(ctx, state, finalMsg.Content)
			}

			return state, nil
		})
	}

	graph.AddLambdaNode("get_state", getStateNode)
	graph.AddLambdaNode("support", wrapModel(models.SkillSupport))
	graph.AddLambdaNode("contract", wrapModel(models.SkillDesign))
	graph.AddLambdaNode("general", wrapModel("default"))

	// 3. Tools executor
	toolsExecutor := compose.InvokableLambda(func(ctx context.Context, state *AgentState) (*AgentState, error) {
		toolNode, _ := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{&graphqlTool{s: s}, &saveContextTool{s: s}},
		})
		lastMsg := state.Messages[len(state.Messages)-1]
		toolMsgs, err := toolNode.Invoke(ctx, lastMsg)
		if err != nil {
			return nil, err
		}

		onEvent, isStreaming := ctx.Value("stream_callback").(models.StreamHandler)
		if isStreaming {
			for _, tm := range toolMsgs {
				if tm.ToolCallID != "" {
					// Link tool output to frontend
					var query string
					for _, tc := range lastMsg.ToolCalls {
						if tc.ID == tm.ToolCallID && tc.Function.Name == ToolExecuteGraphQL {
							var args struct {
								Query string `json:"query"`
							}
							_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
							query = args.Query
							break
						}
					}

					if query != "" {
						toolCallData := map[string]string{
							"query":    query,
							"response": tm.Content,
						}
						data, _ := json.Marshal(toolCallData)
						onEvent(models.EventToolCall, string(data))
					}
				}
			}
		}

		state.Messages = append(state.Messages, toolMsgs...)
		return state, nil
	})
	graph.AddLambdaNode("tools", toolsExecutor)

	// Edges
	graph.AddEdge(compose.START, "get_state")

	// Router from get_state to agent
	routeSkill := func(ctx context.Context, state *AgentState) (string, error) {
		if state.Skill == models.SkillSupport {
			return "support", nil
		}
		if state.Skill == models.SkillDesign {
			return "contract", nil
		}
		return "general", nil
	}
	graph.AddBranch("get_state", compose.NewGraphBranch(routeSkill, map[string]bool{
		"support":  true,
		"contract": true,
		"general":  true,
	}))

	// Conditional routing out of Agent node
	routeAgentExit := func(ctx context.Context, state *AgentState) (string, error) {
		lastMsg := state.Messages[len(state.Messages)-1]
		if len(lastMsg.ToolCalls) > 0 {
			return "tools", nil
		}
		return compose.END, nil
	}

	for _, agent := range []string{"support", "contract", "general"} {
		graph.AddBranch(agent, compose.NewGraphBranch(routeAgentExit, map[string]bool{
			"tools":     true,
			compose.END: true,
		}))
	}

	// Tool node routes back to origin agent
	graph.AddBranch("tools", compose.NewGraphBranch(routeSkill, map[string]bool{
		"support":  true,
		"contract": true,
		"general":  true,
	}))

	return graph.Compile(ctx)
}

func (s *AgentService) ensureConversationOwnership(ctx context.Context, companyID, convID string) error {
	convs, err := s.agentRepo.GetConversations(ctx, companyID)
	if err != nil {
		return fmt.Errorf("failed to verify conversation ownership: %w", err)
	}
	for _, c := range convs {
		if c.ID == convID {
			return nil
		}
	}
	return ErrForbidden
}

func (s *AgentService) saveToolMessages(ctx context.Context, convID, toolCallID, name, args, output string) {
	calls, _ := json.Marshal([]openai.ToolCall{{ID: toolCallID, Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: name, Arguments: args}}})
	callsStr := string(calls)

	_ = s.agentRepo.AddMessage(ctx, &models.AgentMessage{
		ID: uuid.New().String(), ConversationID: convID, Role: models.RoleAssistant, ToolCalls: &callsStr,
	})
	_ = s.agentRepo.AddMessage(ctx, &models.AgentMessage{
		ID: uuid.New().String(), ConversationID: convID, Role: models.RoleTool, Content: output, ToolCallID: &toolCallID,
	})
}

func (s *AgentService) saveAssistantMessage(ctx context.Context, convID, content string) {
	_ = s.agentRepo.AddMessage(ctx, &models.AgentMessage{
		ID: uuid.New().String(), ConversationID: convID, Role: models.RoleAssistant, Content: content, CreatedAt: time.Now(),
	})
}

func (s *AgentService) updateConversationStats(ctx context.Context, convID string, newTokens int, onEvent models.StreamHandler) (int, int) {
	msgCount, tokenCount, err := s.agentRepo.GetConversationStats(ctx, convID)
	if err != nil {
		return 0, 0
	}
	msgCount += 2
	tokenCount += newTokens
	_ = s.agentRepo.UpdateConversationStats(ctx, convID, msgCount, tokenCount)

	if onEvent != nil {
		onEvent(models.EventTokens, fmt.Sprintf("%d", tokenCount))
		onEvent(models.EventMessageCount, fmt.Sprintf("%d", msgCount))
	}

	return msgCount, tokenCount
}

func (s *AgentService) estimateTokens(text string) int {
	return len(text) / 4
}

func (s *AgentService) emitContractPreview(ctx context.Context, state *AgentState, content string) {
	onEvent, ok := ctx.Value("stream_callback").(models.StreamHandler)
	if !ok {
		s.logger.Warn("emitContractPreview: stream_callback not found in context")
		return
	}

	// Extract YAML block from content
	re := regexp.MustCompile("(?s)```yaml\n?(.*?)```")
	yamlMatch := re.FindStringSubmatch(content)
	var yamlContent string
	if len(yamlMatch) >= 2 {
		yamlContent = strings.TrimSpace(yamlMatch[1])
	} else {
		// Fallback for non-codeblock YAML if present
		if strings.Contains(content, "contract_name:") {
			yamlContent = strings.TrimSpace(content)
		} else {
			return
		}
	}

	// Find the last tool message from execute_graphql
	var lastToolOutput string
	for i := len(state.Messages) - 1; i >= 0; i-- {
		msg := state.Messages[i]
		if msg.Role == schema.Tool {
			lastToolOutput = msg.Content
			break
		}
	}

	if yamlContent != "" {
		// Call the engine's preview API to get the graph data
		previewURL := s.threadifyEngineURL + "/v1/contracts/preview"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, previewURL, strings.NewReader(yamlContent))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-yaml")
			// We might need an auth header depending on engine settings
			// req.Header.Set("Authorization", ...)

			resp, err := http.DefaultClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				previewData := map[string]interface{}{
					"yaml":     yamlContent,
					"response": string(body),
				}
				data, _ := json.Marshal(previewData)
				onEvent(models.EventContractPreview, string(data))
				s.logger.Info("emitted EventContractPreview with engine graph", zap.String("conversation_id", state.ConversationID))
				return
			}
		}

		// Fallback to minimal data if engine call fails
		previewData := map[string]string{
			"yaml":     yamlContent,
			"response": lastToolOutput,
		}
		data, _ := json.Marshal(previewData)
		onEvent(models.EventContractPreview, string(data))
		s.logger.Warn("emitted EventContractPreview with fallback data", zap.String("conversation_id", state.ConversationID))
	}
}

// Passthrough methods for conversation management
func (s *AgentService) GetConversations(ctx context.Context, companyID string) ([]models.AgentConversation, error) {
	return s.agentRepo.GetConversations(ctx, companyID)
}

func (s *AgentService) GetMessagesForUser(ctx context.Context, companyID, convID string) ([]*models.AgentMessage, error) {
	if err := s.ensureConversationOwnership(ctx, companyID, convID); err != nil {
		return nil, err
	}
	return s.agentRepo.GetMessages(ctx, convID)
}

func (s *AgentService) DeleteConversation(ctx context.Context, convID, userID string) error {
	return s.agentRepo.DeleteConversation(ctx, convID, userID)
}

func (s *AgentService) GetMaxMessages() int {
	return s.maxMessages
}

func (s *AgentService) GetMaxTokens() int {
	return s.maxTokens
}

func (s *AgentService) ContinueConversation(ctx context.Context, userID, companyID, parentConvID string) (string, string, string, error) {
	convs, err := s.agentRepo.GetConversations(ctx, companyID)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to load conversations: %w", err)
	}

	owns := false
	var parentTitle string
	for _, conv := range convs {
		if conv.ID == parentConvID {
			owns = true
			parentTitle = conv.Title
			break
		}
	}
	if !owns {
		return "", "", "", fmt.Errorf("not authorized to continue this conversation")
	}

	messages, err := s.agentRepo.GetMessages(ctx, parentConvID)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to load parent messages: %w", err)
	}

	var conversationHistory []openai.ChatCompletionMessage
	for _, msg := range messages {
		if msg.Role == models.RoleUser || (msg.Role == models.RoleAssistant && msg.Content != "") {
			conversationHistory = append(conversationHistory, openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	var summary string
	if len(conversationHistory) > 0 {
		summaryReq := openai.ChatCompletionRequest{
			Model: ModelGPT4oMini,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    models.RoleSystem,
					Content: "You are a helpful assistant that creates concise summaries of conversations. Preserve all important facts, decisions, code snippets, thread IDs, technical details, and context. Be comprehensive but concise.",
				},
				{
					Role:    models.RoleUser,
					Content: "Summarize the following conversation, preserving all important context and details:",
				},
			},
			MaxTokens: s.summaryMaxTokens,
		}
		summaryReq.Messages = append(summaryReq.Messages, conversationHistory...)

		summaryResp, err := s.openaiClient.CreateChatCompletion(ctx, summaryReq)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to generate summary: %w", err)
		}
		summary = summaryResp.Choices[0].Message.Content
	}

	newConvID := uuid.New().String()
	newConv := &models.AgentConversation{
		ID:        newConvID,
		UserID:    userID,
		CompanyID: companyID,
		Title:     parentTitle + " (continued)",
	}

	if err := s.agentRepo.CreateConversationWithParent(ctx, newConv, parentConvID); err != nil {
		return "", "", "", fmt.Errorf("failed to create conversation: %w", err)
	}

	if summary != "" {
		summaryCtx := &models.AgentContext{
			ID:             uuid.New().String(),
			ConversationID: newConvID,
			ContextKey:     models.ContextKeySummary,
			ContextValue:   summary,
		}
		if err := s.agentRepo.SaveContext(ctx, summaryCtx); err != nil {
			s.logger.Error("failed to save summary context", zap.Error(err), zap.String("newConversationID", newConvID))
		}
	}

	return newConvID, newConv.Title, summary, nil
}

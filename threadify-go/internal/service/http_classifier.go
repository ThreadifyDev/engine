package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/threadify/engine/pkg/validator"
)

const unmatchedAction = "__none_of_these__"
const minimumChoiceProbability = 0.75

// ClassifierOptions selects either a private Jev-compatible API or the
// license-authenticated Threadify classifier endpoint.
type ClassifierOptions struct {
	BaseURL        string
	Model          string
	APIKeyEnv      string
	BearerToken    string
	InstallationID string
	TimeoutMs      int
}

// HTTPClassifier uses Jev's typed Choice request and response contract.
// The classifier only selects from caller-provided labels. Contract eligibility
// is evaluated separately and remains authoritative.
type HTTPClassifier struct {
	Endpoint       string
	Model          string
	KeyEnv         string
	BearerToken    string
	InstallationID string
	Client         *http.Client
}

func NewHTTPClassifier(options ClassifierOptions) (*HTTPClassifier, error) {
	u, err := url.Parse(strings.TrimRight(options.BaseURL, "/"))
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("classifier.base_url is invalid")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, fmt.Errorf("classifier.base_url requires HTTPS except on loopback")
	}
	if strings.TrimSpace(options.Model) == "" {
		return nil, fmt.Errorf("classifier.model is required")
	}
	if options.TimeoutMs == 0 {
		options.TimeoutMs = 5000
	}
	if options.TimeoutMs < 1 || options.TimeoutMs > 30000 {
		return nil, fmt.Errorf("classifier timeout is invalid")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &HTTPClassifier{
		Endpoint: strings.TrimRight(u.String(), "/") + "/systemone", Model: options.Model,
		KeyEnv: options.APIKeyEnv, BearerToken: options.BearerToken,
		InstallationID: options.InstallationID,
		Client: &http.Client{Timeout: time.Duration(options.TimeoutMs) * time.Millisecond, Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

type jevQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type jevChoice struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
}

func (c *HTTPClassifier) MatchAction(ctx context.Context, goal string, actions []string) (string, error) {
	if len(actions) == 0 || len(actions) > 254 {
		return "", fmt.Errorf("contract action count is outside classifier limits")
	}
	criteria := make(map[string]string, len(actions)+1)
	for _, action := range actions {
		if action == unmatchedAction {
			return "", fmt.Errorf("contract action uses reserved classifier label")
		}
		criteria[action] = strings.ReplaceAll(action, "_", " ")
	}
	criteria[unmatchedAction] = "The goal does not clearly request exactly one of the other actions"
	answer, err := c.choice(ctx, goal, jevQuestion{
		Type: "choice", Instructions: "Which authored contract action does the requested goal describe? Choose none if ambiguous or unrelated.", Criteria: criteria,
	})
	if err != nil {
		return "", err
	}
	if answer.Choice == unmatchedAction || answer.Probabilities[answer.Choice] < minimumChoiceProbability {
		return "", nil
	}
	if _, ok := criteria[answer.Choice]; !ok {
		return "", fmt.Errorf("classifier selected an unlisted action")
	}
	return answer.Choice, nil
}

func (c *HTTPClassifier) Recommend(ctx context.Context, action string, facts DecisionSnapshot) (string, string, error) {
	state := map[string]any{"candidate_action": action, "thread": facts}
	answer, err := c.choice(ctx, state, jevQuestion{
		Type: "choice", Instructions: "Given this process state, should the agent choose this eligible candidate action now?",
		Criteria: map[string]string{"yes": "The action fits the current process state", "no": "Another eligible action better fits the current process state", "uncertain": "The state does not support a clear recommendation"},
	})
	if err != nil {
		return "", "", err
	}
	choice := answer.Choice
	probability := answer.Probabilities[choice]
	if probability < minimumChoiceProbability {
		choice = "uncertain"
	}
	return choice, fmt.Sprintf("classifier selected %s with probability %.2f", answer.Choice, probability), nil
}

// ReviewComposition asks for a bounded, advisory semantic check. The choice
// API cannot prove inconsistency, so only a high-confidence conflict is shown
// as a warning in preview; it never makes a valid contract unpublishable.
func (c *HTTPClassifier) ReviewComposition(ctx context.Context, source string, expanded *validator.Contract) (bool, error) {
	answer, err := c.choice(ctx, map[string]any{"authored_source": source, "expanded_contract": expanded}, jevQuestion{
		Type:         "choice",
		Instructions: "Review the authored contract and its included rules. Is there a concrete semantic contradiction that would make the combined process impossible? Only flag a conflict when the rules cannot all hold. Independent branches and different wording are not conflicts.",
		Criteria: map[string]string{
			"potential_conflict": "A concrete contradiction appears between the combined rules",
			"consistent":         "The combined rules appear compatible",
			"uncertain":          "The information does not establish compatibility or contradiction",
		},
	})
	if err != nil {
		return false, err
	}
	return answer.Choice == "potential_conflict" && answer.Probabilities[answer.Choice] >= minimumChoiceProbability, nil
}

func (c *HTTPClassifier) choice(ctx context.Context, state any, question jevQuestion) (jevChoice, error) {
	var empty jevChoice
	raw, err := c.evaluate(ctx, state, question)
	if err != nil {
		return empty, err
	}
	var answer jevChoice
	if err := json.Unmarshal(raw, &answer); err != nil || answer.Type != "choice" {
		return empty, fmt.Errorf("classifier returned no choice decision")
	}
	if _, ok := question.Criteria[answer.Choice]; !ok {
		return empty, fmt.Errorf("classifier selected an unlisted label")
	}
	probability, ok := answer.Probabilities[answer.Choice]
	if !ok || probability < 0 || probability > 1 || len(answer.Probabilities) != len(question.Criteria) {
		return empty, fmt.Errorf("classifier returned invalid probabilities")
	}
	total, highest := 0.0, -1.0
	for label := range question.Criteria {
		p, present := answer.Probabilities[label]
		if !present || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return empty, fmt.Errorf("classifier returned invalid probabilities")
		}
		if p > highest {
			highest = p
		}
		total += p
	}
	if math.Abs(total-1) > 0.02 || probability < highest {
		return empty, fmt.Errorf("classifier returned inconsistent probabilities")
	}
	return answer, nil
}

func (c *HTTPClassifier) EvaluateAssertion(ctx context.Context, state any, question string) (float64, error) {
	if strings.TrimSpace(question) == "" {
		return 0, fmt.Errorf("semantic question is required")
	}
	raw, err := c.evaluate(ctx, state, map[string]string{"type": "noul", "instructions": question})
	if err != nil {
		return 0, err
	}
	var answer struct {
		Type string   `json:"type"`
		Noul *float64 `json:"noul"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil || answer.Type != "noul" || answer.Noul == nil || math.IsNaN(*answer.Noul) || *answer.Noul < 0 || *answer.Noul > 1 {
		return 0, fmt.Errorf("classifier returned an invalid yes/no probability")
	}
	return *answer.Noul, nil
}

func (c *HTTPClassifier) evaluate(ctx context.Context, state any, question any) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]any{"model": c.Model, "state": state, "questions": map[string]any{"decision": question}})
	if err != nil {
		return nil, err
	}
	if len(payload) > 16384 {
		return nil, fmt.Errorf("classifier input is too large")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	key := c.BearerToken
	if c.KeyEnv != "" {
		key = os.Getenv(c.KeyEnv)
		if key == "" {
			return nil, fmt.Errorf("classifier credential is unavailable")
		}
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if c.InstallationID != "" {
		req.Header.Set("X-Threadify-Installation-ID", c.InstallationID)
	}
	response, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("classifier request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("classifier returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(body) > 65536 {
		return nil, fmt.Errorf("classifier response is unavailable or too large")
	}
	var result struct {
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("classifier returned invalid JSON")
	}
	answer, ok := result.Answers["decision"]
	if !ok || len(answer) == 0 {
		return nil, fmt.Errorf("classifier returned no decision")
	}
	return answer, nil
}

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/pkg/validator"
)

type classifierTransport func(*http.Request) (*http.Response, error)

func (f classifierTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestHTTPClassifierParsesOnlyBoundedLabels(t *testing.T) {
	c, err := NewHTTPClassifier(ClassifierOptions{BaseURL: "https://classifier.example/v1", Model: "jev-latest", TimeoutMs: 1000})
	require.NoError(t, err)
	c.Client.Transport = classifierTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/v1/systemone", req.URL.Path)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"answers":{"decision":{"type":"choice","choice":"invented","probabilities":{"invented":0.9}}}}`)), Header: make(http.Header)}, nil
	})
	_, err = c.MatchAction(context.Background(), "I want a refund", []string{"issue_refund"})
	require.ErrorContains(t, err, "unlisted label")
	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{"issue_refund": {Type: "step"}}}}
	_, _, err = ResolveAction(context.Background(), graph, "", "I want a refund", c)
	require.ErrorContains(t, err, "unlisted label")
}

func TestHTTPClassifierCompositionReviewIsAdvisory(t *testing.T) {
	c, err := NewHTTPClassifier(ClassifierOptions{BaseURL: "https://classifier.example/v1", Model: "jev-latest"})
	require.NoError(t, err)
	c.Client.Transport = classifierTransport(func(req *http.Request) (*http.Response, error) {
		var request struct {
			State struct {
				AuthoredSource   string             `json:"authored_source"`
				ExpandedContract validator.Contract `json:"expanded_contract"`
			} `json:"state"`
			Questions map[string]struct {
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		require.NoError(t, json.NewDecoder(req.Body).Decode(&request))
		require.Contains(t, request.State.AuthoredSource, "Include:")
		require.Equal(t, "refund", request.State.ExpandedContract.ContractName)
		require.Contains(t, request.Questions["decision"].Criteria, "potential_conflict")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"answers":{"decision":{"type":"choice","choice":"potential_conflict","probabilities":{"potential_conflict":0.85,"consistent":0.05,"uncertain":0.10}}}}`)), Header: make(http.Header)}, nil
	})
	flagged, err := c.ReviewComposition(context.Background(), "Feature: refund\nInclude: identity:2", &validator.Contract{ContractName: "refund"})
	require.NoError(t, err)
	require.True(t, flagged)
}

func TestHTTPClassifierUsesJevChoiceAndWithholdsLowConfidenceMatch(t *testing.T) {
	c, err := NewHTTPClassifier(ClassifierOptions{BaseURL: "https://classifier.example/v1", Model: "jev-latest", BearerToken: "license", InstallationID: "installation", TimeoutMs: 1000})
	require.NoError(t, err)
	c.Client.Transport = classifierTransport(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "Bearer license", req.Header.Get("Authorization"))
		require.Equal(t, "installation", req.Header.Get("X-Threadify-Installation-ID"))
		var request struct {
			Model     string `json:"model"`
			State     string `json:"state"`
			Questions map[string]struct {
				Type     string            `json:"type"`
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		require.NoError(t, json.NewDecoder(req.Body).Decode(&request))
		require.Equal(t, "jev-latest", request.Model)
		require.Equal(t, "I want to process a refund", request.State)
		require.Equal(t, "choice", request.Questions["decision"].Type)
		require.Contains(t, request.Questions["decision"].Criteria, "issue_refund")
		require.Contains(t, request.Questions["decision"].Criteria, unmatchedAction)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"answers":{"decision":{"type":"choice","choice":"issue_refund","probabilities":{"issue_refund":0.7,"__none_of_these__":0.3}}}}`)), Header: make(http.Header)}, nil
	})
	matched, err := c.MatchAction(context.Background(), "I want to process a refund", []string{"issue_refund"})
	require.NoError(t, err)
	require.Empty(t, matched)
}

func TestHTTPClassifierEvaluatesSemanticNoul(t *testing.T) {
	c, err := NewHTTPClassifier(ClassifierOptions{BaseURL: "https://classifier.example/v1", Model: "jev-latest"})
	require.NoError(t, err)
	c.Client.Transport = classifierTransport(func(req *http.Request) (*http.Response, error) {
		var request map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(req.Body).Decode(&request))
		require.Contains(t, string(request["questions"]), `"type":"noul"`)
		require.Contains(t, string(request["state"]), "customer-7")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"answers":{"decision":{"type":"noul","noul":0.92}}}`)), Header: make(http.Header)}, nil
	})
	probability, err := c.EvaluateAssertion(context.Background(), map[string]string{"customer": "customer-7"}, "Does this match the verified customer?")
	require.NoError(t, err)
	require.Equal(t, 0.92, probability)
}

package utils

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/threadify/engine/internal/models"
)

// CytoscapeElement represents a single element (node or edge) in Cytoscape format
type CytoscapeElement struct {
	Group string                 `json:"group"` // "nodes" or "edges"
	Data  map[string]interface{} `json:"data"`
}

// CytoscapeGraph represents the complete graph structure for Cytoscape.js
type CytoscapeGraph struct {
	Elements []CytoscapeElement `json:"elements"`
}

// formatLabel formats node IDs with proper text-to-node ratio to prevent overflow
func formatLabel(id string) string {
	// Replace underscores with spaces
	formatted := strings.ReplaceAll(id, "_", " ")

	// Capitalize first letter of each word
	words := strings.Fields(formatted)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	formatted = strings.Join(words, " ")

	// Calculate appropriate length for node dimensions
	// For 120-140px width with 13-14px font, aim for 12-15 characters max
	const maxChars = 15

	if len(formatted) > maxChars {
		// Try to break at word boundaries first
		wordList := strings.Split(formatted, " ")
		var result strings.Builder
		currentLength := 0

		for _, word := range wordList {
			if currentLength == 0 {
				if len(word) <= maxChars-3 {
					result.WriteString(word)
					currentLength = len(word)
				} else {
					// Single word too long, truncate it
					result.WriteString(word[:maxChars-3])
					break
				}
			} else if currentLength+len(word)+1 <= maxChars-3 { // -3 for "..."
				result.WriteString(" ")
				result.WriteString(word)
				currentLength += len(word) + 1
			} else {
				break
			}
		}

		if result.Len() > 0 {
			formatted = result.String() + "..."
		} else {
			// If no words fit, truncate first word
			if len(formatted) > maxChars-3 {
				formatted = formatted[:maxChars-3] + "..."
			}
		}
	}

	return formatted
}

// ContractGraphToCytoscape converts a ContractGraph to Cytoscape.js format
func ContractGraphToCytoscape(contractName string, graph *models.ContractGraph) *CytoscapeGraph {
	var elements []CytoscapeElement

	// Add nodes
	for id, node := range graph.Graph.Nodes {
		nodeData := map[string]interface{}{
			"id":   id,
			"type": determineNodeType(id, graph),
		}

		// Create properly formatted label with text-to-node ratio
		label := formatLabel(id)
		nodeData["label"] = label

		// Add additional metadata
		nodeData["owner"] = node.Owner
		nodeData["timeout"] = node.Timeout
		if node.Type != "" {
			nodeData["nodeType"] = node.Type
		}
		if node.Mode != "" {
			nodeData["mode"] = node.Mode
		}

		elements = append(elements, CytoscapeElement{
			Group: "nodes",
			Data:  nodeData,
		})
	}

	// Add edges (transitions)
	for _, transition := range graph.Transitions {
		for _, target := range transition.To {
			edgeData := map[string]interface{}{
				"source": transition.From,
				"target": target,
			}

			// Add retry label if configured
			if transition.CanRetry && transition.MaxRetries > 0 {
				edgeData["label"] = fmt.Sprintf("retry: %d", transition.MaxRetries)
				edgeData["canRetry"] = true
				edgeData["maxRetries"] = transition.MaxRetries
			}

			elements = append(elements, CytoscapeElement{
				Group: "edges",
				Data:  edgeData,
			})
		}
	}

	return &CytoscapeGraph{
		Elements: elements,
	}
}

// determineNodeType classifies a node as entry, step, or terminal
func determineNodeType(nodeID string, graph *models.ContractGraph) string {
	// Check if it's an entry point
	for _, entry := range graph.Graph.EntryPoints {
		if entry == nodeID {
			return "entry"
		}
	}

	// Check if it's a terminal step
	for _, terminal := range graph.Graph.TerminalSteps {
		if terminal == nodeID {
			return "terminal"
		}
	}

	// Default to step
	return "step"
}

// ContractGraphToCytoscapeJSON converts a ContractGraph to Cytoscape.js JSON string
func ContractGraphToCytoscapeJSON(contractName string, graph *models.ContractGraph) (string, error) {
	cytoscapeGraph := ContractGraphToCytoscape(contractName, graph)

	// Convert to JSON
	jsonBytes, err := json.Marshal(cytoscapeGraph)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

package utils

import (
	"fmt"
	"strings"

	"github.com/threadify/engine/internal/models"
)

// ContractGraphToMermaid converts a ContractGraph to Mermaid flowchart syntax
func ContractGraphToMermaid(contractName string, graph *models.ContractGraph) string {
	var builder strings.Builder

	// Header
	builder.WriteString(fmt.Sprintf("flowchart TD\n"))
	builder.WriteString(fmt.Sprintf("\n"))

	// Define styles for different node types
	builder.WriteString("    %% Node styles\n")
	builder.WriteString("    classDef entry fill:#e1f5fe,stroke:#01579b,stroke-width:2px\n")
	builder.WriteString("    classDef terminal fill:#f3e5f5,stroke:#4a148c,stroke-width:2px\n")
	builder.WriteString("    classDef step fill:#e8f5e8,stroke:#2e7d32,stroke-width:2px\n")
	builder.WriteString("    classDef group fill:#fff3e0,stroke:#e65100,stroke-width:2px\n")

	// Group nodes by party for swim lanes
	parties := getParties(graph)

	// Create swim lanes for each party
	builder.WriteString("    %% Swim lanes by party\n")
	for _, party := range parties {
		builder.WriteString(fmt.Sprintf("    subgraph %s\n", party))

		// Add nodes belonging to this party
		for id, node := range graph.Graph.Nodes {
			if node.Owner == party && node.Type != "parallel_group" {
				nodeID := sanitizeNodeID(id)

				// Create enhanced node label with timeout
				label := fmt.Sprintf("%s", id)
				if node.Timeout != "" {
					label += fmt.Sprintf("\\n⏱️ %s", node.Timeout)
				}
				label += fmt.Sprintf("\\n(%s)", node.Owner)

				builder.WriteString(fmt.Sprintf("        %s[\"%s\"]\n", nodeID, label))
			}
		}

		builder.WriteString("    end\n")
	}

	// Handle parallel groups outside swim lanes
	builder.WriteString("    %% Parallel groups (outside swim lanes)\n")
	for id, node := range graph.Graph.Nodes {
		if node.Type == "parallel_group" {
			nodeID := sanitizeNodeID(id)

			// Create group node with enhanced info
			label := fmt.Sprintf("Group: %s\\n(%s)", id, node.Mode)
			if node.MaxDuration != "" {
				label += fmt.Sprintf("\\n⏱️ %s", node.MaxDuration)
			}

			builder.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", nodeID, label))

			// Add subgraph for parallel group steps
			builder.WriteString(fmt.Sprintf("    subgraph %s_steps[\"%s steps\"]\n", nodeID, id))
			for _, stepID := range node.Steps {
				if stepNode, exists := graph.Graph.Nodes[stepID]; exists {
					stepNodeID := sanitizeNodeID(stepID)
					stepLabel := stepID
					if stepNode.Timeout != "" {
						stepLabel += fmt.Sprintf("\\n⏱️ %s", stepNode.Timeout)
					}
					builder.WriteString(fmt.Sprintf("        %s[\"%s\"]\n", stepNodeID, stepLabel))
				}
			}
			builder.WriteString("    end\n")
		}
	}

	// Create node ID mapping
	nodeIDs := make(map[string]string)
	for id := range graph.Graph.Nodes {
		nodeIDs[id] = sanitizeNodeID(id)
	}

	// Add transitions (connections)
	builder.WriteString("    %% Transitions\n")
	for _, transition := range graph.Transitions {
		fromNode := nodeIDs[transition.From]
		for _, to := range transition.To {
			toNode := nodeIDs[to]

			// Add transition label if retry is configured
			label := ""
			if transition.CanRetry {
				label = fmt.Sprintf("| retry: %d |", transition.MaxRetries)
			}

			builder.WriteString(fmt.Sprintf("    %s --> %s %s\n", fromNode, toNode, label))
		}
	}

	// Add class assignments
	builder.WriteString("    %% Node classifications\n")

	// Entry points
	for _, entry := range graph.Graph.EntryPoints {
		if nodeID, exists := nodeIDs[entry]; exists {
			builder.WriteString(fmt.Sprintf("    class %s entry\n", nodeID))
		}
	}

	// Terminal steps
	for _, terminal := range graph.Graph.TerminalSteps {
		if nodeID, exists := nodeIDs[terminal]; exists {
			builder.WriteString(fmt.Sprintf("    class %s terminal\n", nodeID))
		}
	}

	// Regular steps vs groups
	for id, node := range graph.Graph.Nodes {
		nodeID := nodeIDs[id]
		if node.Type == "parallel_group" {
			builder.WriteString(fmt.Sprintf("    class %s group\n", nodeID))
		} else if !isEntryPoint(id, graph.Graph.EntryPoints) && !isTerminal(id, graph.Graph.TerminalSteps) {
			builder.WriteString(fmt.Sprintf("    class %s step\n", nodeID))
		}
	}

	return builder.String()
}

// getParties returns a list of unique parties in the graph
func getParties(graph *models.ContractGraph) []string {
	parties := make(map[string]bool)
	for _, node := range graph.Graph.Nodes {
		parties[node.Owner] = true
	}
	var partyList []string
	for party := range parties {
		partyList = append(partyList, party)
	}
	return partyList
}

// sanitizeNodeID converts step IDs to valid Mermaid node identifiers
func sanitizeNodeID(id string) string {
	// Replace invalid characters with underscores
	sanitized := strings.ReplaceAll(id, "-", "_")
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	sanitized = strings.ReplaceAll(sanitized, " ", "_")
	return "node_" + sanitized
}

// isEntryPoint checks if a step is an entry point
func isEntryPoint(stepID string, entryPoints []string) bool {
	for _, entry := range entryPoints {
		if entry == stepID {
			return true
		}
	}
	return false
}

// isTerminal checks if a step is a terminal step
func isTerminal(stepID string, terminalSteps []string) bool {
	for _, terminal := range terminalSteps {
		if terminal == stepID {
			return true
		}
	}
	return false
}

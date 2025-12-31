package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/utils"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: mermaid-converter <contract-graph-json-file>")
		os.Exit(1)
	}

	// Read contract graph JSON file
	filename := os.Args[1]
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("Failed to read file %s: %v", filename, err)
	}

	// Parse contract graph
	var graph models.ContractGraph
	if err := json.Unmarshal(data, &graph); err != nil {
		log.Fatalf("Failed to parse contract graph: %v", err)
	}

	// Convert to Mermaid
	contractName := "Sample Contract"
	if len(os.Args) > 2 {
		contractName = os.Args[2]
	}

	mermaidCode := utils.ContractGraphToMermaid(contractName, &graph)
	fmt.Println(mermaidCode)
}

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"golf-rules-rag/internal/database"
	"golf-rules-rag/internal/embedding"
	"golf-rules-rag/internal/llm"
	"golf-rules-rag/internal/models"
)

const (
	DefaultContextLimit = 5
)

func main() {
	// Parse command line flags
	pgConnString := flag.String("pg", "postgres://golfrag:golfrag@localhost:5432/golfrag?sslmode=disable", "PostgreSQL connection string")
	ollamaHost := flag.String("ollama", "", "Ollama host (default uses OLLAMA_HOST env var)")
	model := flag.String("model", "phi3-mini", "Ollama model for answering")
	embeddingModel := flag.String("embedding-model", "phi3-mini", "Ollama model for embeddings")
	contextLimit := flag.Int("context", DefaultContextLimit, "Number of similar contexts to retrieve")
	interactive := flag.Bool("i", false, "Run in interactive mode")
	queryFlag := flag.String("q", "", "Query to answer (non-interactive mode)")
	ruleFilter := flag.String("rule", "", "Filter by rule number (e.g., 'Rule 14')")
	listRules := flag.Bool("list-rules", false, "List all available rule sections")
	flag.Parse()

	// Create context
	ctx := context.Background()

	// Connect to database
	db, err := database.NewDB(*pgConnString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// List rules if requested
	if *listRules {
		sections, err := db.GetRuleSections(ctx)
		if err != nil {
			log.Fatalf("Failed to get rule sections: %v", err)
		}

		fmt.Println("Available Rule Sections:")
		for _, section := range sections {
			fmt.Println("  " + section)
		}
		return
	}

	// Create embedder
	embedder, err := embedding.NewOllamaEmbedder(*ollamaHost, *embeddingModel)
	if err != nil {
		log.Fatalf("Failed to create embedder: %v", err)
	}

	// Create LLM
	llmClient, err := llm.NewOllamaLLM(*ollamaHost, *model)
	if err != nil {
		log.Fatalf("Failed to create LLM client: %v", err)
	}

	if *interactive {
		runInteractiveMode(ctx, db, embedder, llmClient, *contextLimit, *ruleFilter)
	} else {
		if *queryFlag == "" {
			log.Fatal("Query is required in non-interactive mode. Use -q 'your question'")
		}

		// Process a single query
		answer, err := processQuery(ctx, *queryFlag, db, embedder, llmClient, *contextLimit, *ruleFilter)
		if err != nil {
			log.Fatalf("Failed to process query: %v", err)
		}

		fmt.Println(formatAnswer(answer))
	}
}

func runInteractiveMode(ctx context.Context, db *database.DB, embedder *embedding.OllamaEmbedder,
	llmClient *llm.OllamaLLM, contextLimit int, ruleFilter string) {

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Golf Rules Assistant - Ask questions about golf rules (type 'exit' to quit)")
	if ruleFilter != "" {
		fmt.Printf("Filtering results to rules matching: %s\n", ruleFilter)
	}

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		if strings.ToLower(input) == "exit" || strings.ToLower(input) == "quit" {
			break
		}

		if strings.TrimSpace(input) == "" {
			continue
		}

		// Check for command to set rule filter
		if strings.HasPrefix(strings.ToLower(input), "/rule ") {
			ruleFilter = strings.TrimSpace(strings.TrimPrefix(input, "/rule "))
			if ruleFilter == "" {
				fmt.Println("Rule filter cleared")
			} else {
				fmt.Printf("Rule filter set to: %s\n", ruleFilter)
			}
			continue
		}

		// Check for command to list rules
		if strings.ToLower(input) == "/list-rules" {
			sections, err := db.GetRuleSections(ctx)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}

			fmt.Println("Available Rule Sections:")
			for _, section := range sections {
				fmt.Println("  " + section)
			}
			continue
		}

		// Show "thinking" indicator
		fmt.Print("Searching golf rules... ")

		answer, err := processQuery(ctx, input, db, embedder, llmClient, contextLimit, ruleFilter)
		if err != nil {
			fmt.Printf("\rError: %v\n", err)
			continue
		}

		fmt.Println("\r" + formatAnswer(answer))
	}
}

func processQuery(ctx context.Context, query string, db *database.DB, embedder *embedding.OllamaEmbedder, llmClient *llm.OllamaLLM, contextLimit int, ruleFilter string) (*models.Response, error) {
	// Extract rule references and golf terms
	queryRuleRefs := extractRuleReferences(query)
	golfTerms := identifyGolfTerms(query)

	// Create embedding for query
	startTime := time.Now()
	queryEmbedding, err := embedder.EmbedText(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to create query embedding: %w", err)
	}

	// Use optimized query function
	var chunks []models.TextChunk
	if len(queryRuleRefs) > 0 || ruleFilter != "" {
		// Use rule-specific querying
		chunks, err = db.QuerySimilarWithStructure(ctx, queryEmbedding, query, contextLimit)
	} else if len(golfTerms) > 0 {
		// Use term-based querying for golf-specific terms
		chunks, err = db.QuerySimilarWithTerms(ctx, queryEmbedding, golfTerms, contextLimit)
	} else {
		// Fall back to regular similarity search
		chunks, err = db.QuerySimilar(ctx, queryEmbedding, contextLimit)
	}

	if len(chunks) == 0 {
		// No relevant context found
		return &models.Response{
			Answer:    "I couldn't find any relevant information in the golf rules to answer your question.",
			Sources:   []models.TextChunk{},
			Timestamp: time.Now().Format(time.RFC3339),
		}, nil
	}

	// Generate answer using LLM
	response, err := llmClient.Answer(ctx, query, chunks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate answer: %w", err)
	}

	elapsedTime := time.Since(startTime)
	log.Printf("Query processed in %v", elapsedTime)

	return response, nil
}

func formatAnswer(response *models.Response) string {
	var sb strings.Builder

	// Add the answer
	sb.WriteString(response.Answer)
	sb.WriteString("\n\n")

	// Add sources if available
	if len(response.Sources) > 0 {
		sb.WriteString("Sources:\n")
		for i, source := range response.Sources {
			section := source.Metadata.Section
			if section == "" {
				section = "N/A"
			}

			title := source.Metadata.Title
			if title == "" {
				title = "N/A"
			}

			sb.WriteString(fmt.Sprintf("  %d. [Section: %s - %s, Page: %d]\n",
				i+1, section, title, source.Metadata.PageNumber))
		}
	}

	return sb.String()
}

// identifyGolfTerms extracts golf-specific terms from the query
func identifyGolfTerms(query string) []string {
	var terms []string

	// Golf-specific term patterns
	patterns := map[string]string{
		"penalty area":        "penalty area",
		"bunker":              "bunker",
		"putting green":       "putting green",
		"teeing area":         "teeing area",
		"loose impediment":    "loose impediment",
		"obstruction":         "obstruction",
		"out of bounds":       "out of bounds",
		"OB":                  "out of bounds",
		"unplayable":          "unplayable ball",
		"stroke and distance": "stroke-and-distance",
		// Add more golf terms...
	}

	for pattern, term := range patterns {
		if strings.Contains(strings.ToLower(query), pattern) {
			terms = append(terms, term)
		}
	}

	return terms
}

// extractRuleReferences extracts rule references from a query
func extractRuleReferences(query string) []string {
	rulePattern := regexp.MustCompile(`Rule\s+(\d+)(\.\d+)?([a-z])?(\(\d+\))?`)
	matches := rulePattern.FindAllStringSubmatch(query, -1)

	var ruleRefs []string
	for _, match := range matches {
		if len(match) > 0 {
			// Full match is at index 0
			ruleRef := match[0]
			ruleRefs = append(ruleRefs, ruleRef)

			// Also add the main rule number for broader context
			if len(match) > 1 && match[1] != "" {
				mainRule := "Rule " + match[1]
				if !contains(ruleRefs, mainRule) {
					ruleRefs = append(ruleRefs, mainRule)
				}
			}
		}
	}

	return ruleRefs
}

// contains checks if a string slice contains a specific value
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

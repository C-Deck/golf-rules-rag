package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golf-rules-rag/internal/database"
	"golf-rules-rag/internal/embedding"
	"golf-rules-rag/internal/llm"
	"golf-rules-rag/internal/models"
)

const (
	DEFAULT_CONTEXT_LIMIT = 5
)

func main() {
	// Parse command line flags
	pgConnString := flag.String("pg", "postgres://golfrag:golfrag@localhost:5432/golfrag?sslmode=disable", "PostgreSQL connection string")
	ollamaHost := flag.String("ollama", "", "Ollama host (default uses OLLAMA_HOST env var)")
	model := flag.String("model", "phi3-mini", "Ollama model for answering")
	embeddingModel := flag.String("embedding-model", "phi3-mini", "Ollama model for embeddings")
	contextLimit := flag.Int("context", DEFAULT_CONTEXT_LIMIT, "Number of similar contexts to retrieve")
	interactive := flag.Bool("i", false, "Run in interactive mode")
	queryFlag := flag.String("q", "", "Query to answer (non-interactive mode)")
	flag.Parse()

	// Create context
	ctx := context.Background()

	// Connect to database
	db, err := database.NewDB(*pgConnString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

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
		runInteractiveMode(ctx, db, embedder, llmClient, *contextLimit)
	} else {
		if *queryFlag == "" {
			log.Fatal("Query is required in non-interactive mode. Use -q 'your question'")
		}

		// Process a single query
		answer, err := processQuery(ctx, *queryFlag, db, embedder, llmClient, *contextLimit)
		if err != nil {
			log.Fatalf("Failed to process query: %v", err)
		}

		fmt.Println(formatAnswer(answer))
	}
}

func runInteractiveMode(ctx context.Context, db *database.DB, embedder *embedding.OllamaEmbedder, llmClient *llm.OllamaLLM, contextLimit int) {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Golf Rules Assistant - Ask questions about golf rules (type 'exit' to quit)")

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}

		query := scanner.Text()
		if strings.ToLower(query) == "exit" || strings.ToLower(query) == "quit" {
			break
		}

		if strings.TrimSpace(query) == "" {
			continue
		}

		// Show "thinking" indicator
		fmt.Print("Searching golf rules... ")

		answer, err := processQuery(ctx, query, db, embedder, llmClient, contextLimit)
		if err != nil {
			fmt.Printf("\rError: %v\n", err)
			continue
		}

		fmt.Println("\r" + formatAnswer(answer))
	}
}

func processQuery(ctx context.Context, query string, db *database.DB, embedder *embedding.OllamaEmbedder, llmClient *llm.OllamaLLM, contextLimit int) (*models.Response, error) {
	// Create embedding for query
	startTime := time.Now()
	queryEmbedding, err := embedder.EmbedText(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to create query embedding: %w", err)
	}

	// Get similar chunks from database
	chunks, err := db.QuerySimilar(ctx, queryEmbedding, contextLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to query similar chunks: %w", err)
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

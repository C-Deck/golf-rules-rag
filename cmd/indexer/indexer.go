package main

import (
	"context"
	"flag"
	"log"
	"os"
	"runtime"
	"time"

	"golf-rules-rag/internal/database"
	"golf-rules-rag/internal/embedding"
	"golf-rules-rag/internal/models"
	"golf-rules-rag/internal/processor"
)

func main() {
	// Parse command line flags
	pdfPath := flag.String("pdf", "", "Path to PDF file (required)")
	pgConnString := flag.String("pg", "postgres://golfrag:golfrag@localhost:5432/golfrag?sslmode=disable", "PostgreSQL connection string")
	ollamaHost := flag.String("ollama", "", "Ollama host (default uses OLLAMA_HOST env var)")
	embeddingModel := flag.String("model", "phi3-mini", "Ollama model for embeddings")
	chunkSize := flag.Int("chunk-size", 1000, "Character size for text chunks")
	chunkOverlap := flag.Int("chunk-overlap", 200, "Character overlap between chunks")
	maxConcurrent := flag.Int("max-concurrent", runtime.NumCPU()/2, "Maximum concurrent embedding requests")
	flag.Parse()

	// Validate required flags
	if *pdfPath == "" {
		log.Fatal("PDF path is required")
	}

	// Check if file exists
	if _, err := os.Stat(*pdfPath); os.IsNotExist(err) {
		log.Fatalf("PDF file does not exist: %s", *pdfPath)
	}

	log.Printf("Processing PDF: %s", *pdfPath)
	log.Printf("Using model: %s", *embeddingModel)
	log.Printf("Max concurrent requests: %d", *maxConcurrent)

	// Create context
	ctx := context.Background()

	// Connect to database
	db, err := database.NewDB(*pgConnString)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize database schema
	if err := db.Initialize(ctx); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database initialized successfully")

	// Create PDF processor with semantic chunking
	pdfProcessor := processor.NewPDFProcessor(*chunkSize, *chunkOverlap)

	// Process PDF with semantic chunking
	log.Println("Extracting text from PDF with semantic chunking...")
	chunks, err := pdfProcessor.ProcessPDF(ctx, *pdfPath)
	if err != nil {
		log.Fatalf("Failed to process PDF: %v", err)
	}
	log.Printf("Extracted %d semantic chunks from PDF", len(chunks))

	// Create embedder with parallel processing
	embedder, err := embedding.NewOllamaEmbedder(*ollamaHost, *embeddingModel)
	if err != nil {
		log.Fatalf("Failed to create embedder: %v", err)
	}

	// Set max concurrent embedding requests
	embedder.MaxConcurrent = *maxConcurrent

	// Create embeddings for chunks with parallel processing and progress reporting
	log.Println("Creating embeddings with parallel processing...")
	startTime := time.Now()

	// Define progress function
	progressFunc := func(processed, total int) {
		log.Printf("Progress: %d/%d chunks processed (%.1f%%)",
			processed, total, float64(processed)/float64(total)*100)
	}

	// Process embeddings in parallel with progress reporting
	embeddedChunks, err := embedder.EmbedBatchWithProgress(ctx, chunks, progressFunc)
	if err != nil {
		log.Fatalf("Failed to create embeddings: %v", err)
	}

	// Store chunks in database
	log.Println("Storing chunks in database...")
	for _, chunk := range embeddedChunks {
		if err := db.StoreTextChunk(ctx, &chunk); err != nil {
			log.Printf("Warning: Failed to store chunk %d: %v", chunk.ID, err)
		}
	}

	duration := time.Since(startTime)
	log.Printf("Completed embedding and storing %d chunks in %v", len(embeddedChunks), duration)

	// Print statistics about the chunks
	printChunkStatistics(embeddedChunks)
}

// printChunkStatistics prints statistics about the extracted chunks
func printChunkStatistics(chunks []models.TextChunk) {
	var totalLength int
	sectionMap := make(map[string]int)

	for _, chunk := range chunks {
		totalLength += len(chunk.Content)
		sectionMap[chunk.Metadata.Section]++
	}

	avgLength := float64(totalLength) / float64(len(chunks))

	log.Printf("Chunk Statistics:")
	log.Printf("  Total chunks: %d", len(chunks))
	log.Printf("  Average chunk length: %.1f characters", avgLength)
	log.Printf("  Number of sections: %d", len(sectionMap))

	log.Println("  Section breakdown:")
	for section, count := range sectionMap {
		if section == "" {
			section = "Undefined"
		}
		log.Printf("    %s: %d chunks", section, count)
	}
}

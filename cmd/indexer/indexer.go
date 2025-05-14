package main

import (
	"context"
	"flag"
	"log"
	"os"
	"runtime"
	"strings"
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
	extractDefinitions := flag.Bool("definitions", true, "Extract and process definitions section")
	extractIndex := flag.Bool("index", true, "Extract and process index terms")
	hierarchicalChunking := flag.Bool("hierarchical", true, "Use hierarchical chunking based on rule structure")
	extractCrossRefs := flag.Bool("cross-refs", true, "Extract cross-references between rules")
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
	log.Printf("Processing options: definitions=%v, index=%v, hierarchical=%v, cross-refs=%v",
		*extractDefinitions, *extractIndex, *hierarchicalChunking, *extractCrossRefs)

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

	// Create PDF processor with enhanced options
	pdfProcessor := processor.NewPDFProcessor(*chunkSize, *chunkOverlap)

	// Process PDF with enhanced semantic chunking
	log.Println("Extracting text from PDF with semantic chunking...")
	startTime := time.Now()
	chunks, err := pdfProcessor.ProcessPDF(ctx, *pdfPath)
	if err != nil {
		log.Fatalf("Failed to process PDF: %v", err)
	}
	log.Printf("Extracted %d semantic chunks from PDF in %v",
		len(chunks), time.Since(startTime))

	// Create embedder with parallel processing
	embedder, err := embedding.NewOllamaEmbedder(*ollamaHost, *embeddingModel)
	if err != nil {
		log.Fatalf("Failed to create embedder: %v", err)
	}

	// Set max concurrent embedding requests
	embedder.MaxConcurrent = *maxConcurrent

	// Create embeddings for chunks with parallel processing and progress reporting
	log.Println("Creating embeddings with parallel processing...")
	embeddingStart := time.Now()

	// Define progress function
	progressFunc := func(processed, total int) {
		elapsedTime := time.Since(embeddingStart)
		estimatedTotal := elapsedTime * time.Duration(total) / time.Duration(processed)
		estimatedRemaining := estimatedTotal - elapsedTime

		log.Printf("Progress: %d/%d chunks processed (%.1f%%) - Est. remaining: %v",
			processed, total, float64(processed)/float64(total)*100, estimatedRemaining.Round(time.Second))
	}

	// Process embeddings in parallel with progress reporting
	embeddedChunks, err := embedder.EmbedBatchWithProgress(ctx, chunks, progressFunc)
	if err != nil {
		log.Fatalf("Failed to create embeddings: %v", err)
	}

	// Store chunks in database
	log.Println("Storing chunks in database...")
	storeStart := time.Now()
	chunkCount := 0

	for _, chunk := range embeddedChunks {
		if err := db.StoreTextChunk(ctx, &chunk); err != nil {
			log.Printf("Warning: Failed to store chunk %d: %v", chunk.ID, err)
		} else {
			chunkCount++
		}

		// Print progress every 50 chunks
		if chunkCount%50 == 0 {
			log.Printf("Stored %d/%d chunks...", chunkCount, len(embeddedChunks))
		}
	}

	totalDuration := time.Since(startTime)
	storeDuration := time.Since(storeStart)
	embeddingDuration := embeddingStart.Sub(startTime)

	log.Printf("Completed processing in %v:", totalDuration)
	log.Printf("  - PDF processing: %v", embeddingDuration)
	log.Printf("  - Embedding creation: %v", embeddingStart.Sub(startTime))
	log.Printf("  - Database storage: %v", storeDuration)

	// Print enhanced statistics about the chunks
	printEnhancedChunkStatistics(embeddedChunks)
}

// printEnhancedChunkStatistics prints detailed statistics about the extracted chunks
func printEnhancedChunkStatistics(chunks []models.TextChunk) {
	var totalLength int
	sectionMap := make(map[string]int)
	chunkTypeMap := make(map[string]int)
	ruleReferences := make(map[string][]string)
	hierarchyDepthMap := make(map[int]int)

	for _, chunk := range chunks {
		totalLength += len(chunk.Content)

		// Count by section
		if chunk.Metadata.Section != "" {
			sectionMap[chunk.Metadata.Section]++
		}

		// Count by chunk type
		if chunk.Metadata.ChunkType != "" {
			chunkTypeMap[chunk.Metadata.ChunkType]++
		} else {
			chunkTypeMap["unknown"]++
		}

		// Track cross-references
		if len(chunk.CrossReferences) > 0 {
			if chunk.Metadata.Section != "" {
				ruleReferences[chunk.Metadata.Section] = append(
					ruleReferences[chunk.Metadata.Section],
					chunk.CrossReferences...)
			}
		}

		// Count hierarchy depth (by counting separators in hierarchy path)
		if chunk.Metadata.Hierarchy != "" {
			depth := strings.Count(chunk.Metadata.Hierarchy, ">") + 1
			hierarchyDepthMap[depth]++
		} else {
			hierarchyDepthMap[0]++
		}
	}

	avgLength := float64(totalLength) / float64(len(chunks))

	log.Printf("Chunk Statistics:")
	log.Printf("  Total chunks: %d", len(chunks))
	log.Printf("  Average chunk length: %.1f characters", avgLength)
	log.Printf("  Number of sections: %d", len(sectionMap))

	// Print section breakdown
	log.Println("  Section breakdown:")
	for section, count := range sectionMap {
		if section == "" {
			section = "Undefined"
		}
		log.Printf("    %s: %d chunks", section, count)
	}

	// Print chunk type breakdown
	log.Println("  Chunk type breakdown:")
	for chunkType, count := range chunkTypeMap {
		log.Printf("    %s: %d chunks", chunkType, count)
	}

	// Print hierarchy depth stats
	log.Println("  Hierarchy depth breakdown:")
	for depth, count := range hierarchyDepthMap {
		if depth == 0 {
			log.Printf("    No hierarchy: %d chunks", count)
		} else {
			log.Printf("    Depth %d: %d chunks", depth, count)
		}
	}

	// Print cross-reference stats
	totalRefs := 0
	for _, refs := range ruleReferences {
		totalRefs += len(refs)
	}

	log.Printf("  Cross-references: %d total references across %d sections",
		totalRefs, len(ruleReferences))

	// Print sections with most cross-references
	if len(ruleReferences) > 0 {
		log.Println("  Sections with most cross-references:")
		// This is a simplified approach - for a complete implementation,
		// you would sort the sections by reference count
		count := 0
		for section, refs := range ruleReferences {
			if count < 5 { // Show top 5
				log.Printf("    %s: %d references", section, len(refs))
				count++
			}
		}
	}
}

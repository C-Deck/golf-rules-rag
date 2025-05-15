package embedding

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golf-rules-rag/internal/models"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/envconfig"
)

// OllamaEmbedder generates embeddings using Ollama API
type OllamaEmbedder struct {
	Client        *api.Client
	Model         string
	MaxRetries    int
	Timeout       time.Duration
	MaxConcurrent int
}

// NewOllamaEmbedder creates a new Ollama embedder
func NewOllamaEmbedder(host string, model string) (*OllamaEmbedder, error) {
	hostURL := envconfig.Host()
	if host != "" {

	}
	client := api.NewClient(hostURL, http.DefaultClient)

	return &OllamaEmbedder{
		Client:        client,
		Model:         model,
		MaxRetries:    3,
		Timeout:       time.Second * 30,
		MaxConcurrent: 3, // Limit concurrent requests based on hardware
	}, nil
}

// EmbedText generates an embedding for a text
func (e *OllamaEmbedder) EmbedText(ctx context.Context, text string) ([]float64, error) {
	var embedding []float64
	var err error

	// Implement retry logic
	for retries := 0; retries <= e.MaxRetries; retries++ {
		if retries > 0 {
			// Wait before retrying
			time.Sleep(time.Duration(retries) * time.Second)
		}

		embedding, err = e.createEmbedding(ctx, text)
		if err == nil {
			return embedding, nil
		}
	}

	return nil, fmt.Errorf("failed to create embedding after %d retries: %w", e.MaxRetries, err)
}

// createEmbedding is a helper function to create a single embedding
func (e *OllamaEmbedder) createEmbedding(ctx context.Context, text string) ([]float64, error) {
	req := api.EmbeddingRequest{
		Model:   e.Model,
		Prompt:  text,
		Options: map[string]any{},
	}

	// Create a context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()

	// Make the embedding request
	resp, err := e.Client.Embeddings(ctxWithTimeout, &req)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding: %w", err)
	}

	return resp.Embedding, nil
}

// EmbedBatch generates embeddings for multiple texts in parallel
func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, chunks []models.TextChunk) ([]models.TextChunk, error) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, e.MaxConcurrent)

	// Create a mutex to protect access to the chunks slice
	var mu sync.Mutex

	// Track errors
	errChan := make(chan error, len(chunks))

	// Process chunks in parallel
	for i := range chunks {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(i int) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			// Create embedding for this chunk
			embedding, err := e.EmbedText(ctx, chunks[i].Content)
			if err != nil {
				errChan <- fmt.Errorf("failed to embed chunk %d: %w", chunks[i].ID, err)
				return
			}

			// Update the chunk with its embedding
			mu.Lock()
			chunks[i].Embedding = embedding
			mu.Unlock()
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		return nil, err
	}

	return chunks, nil
}

// EmbedBatchWithProgress generates embeddings with progress reporting
func (e *OllamaEmbedder) EmbedBatchWithProgress(ctx context.Context, chunks []models.TextChunk,
	progressFunc func(processed, total int)) ([]models.TextChunk, error) {

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, e.MaxConcurrent)

	// Create a mutex to protect access to the chunks slice and progress counter
	var mu sync.Mutex
	processed := 0
	total := len(chunks)

	// Track errors
	errChan := make(chan error, total)

	// Process chunks in parallel
	for i := range chunks {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(i int) {
			defer func() {
				wg.Done()
				<-semaphore
			}() // Release semaphore

			// Create embedding for this chunk
			embedding, err := e.EmbedText(ctx, chunks[i].Content)
			if err != nil {
				errChan <- fmt.Errorf("failed to embed chunk %d: %w", chunks[i].ID, err)
				return
			}

			// Update the chunk with its embedding
			mu.Lock()
			chunks[i].Embedding = embedding
			processed++
			if progressFunc != nil {
				progressFunc(processed, total)
			}
			mu.Unlock()
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		return nil, err
	}

	return chunks, nil
}

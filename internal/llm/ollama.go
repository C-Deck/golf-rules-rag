package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golf-rules-rag/internal/models"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/envconfig"
)

// OllamaLLM handles interactions with the Ollama LLM API
type OllamaLLM struct {
	Client *api.Client
	Model  string
}

// NewOllamaLLM creates a new Ollama LLM client
func NewOllamaLLM(host string, model string) (*OllamaLLM, error) {
	hostURL := envconfig.Host()
	if host != "" {

	}
	client := api.NewClient(hostURL, http.DefaultClient)

	return &OllamaLLM{
		Client: client,
		Model:  model,
	}, nil
}

// GeneratePrompt creates a prompt for the LLM with context from the database
func (o *OllamaLLM) GeneratePrompt(query string, contexts []models.TextChunk) string {
	var promptBuilder strings.Builder

	// System instruction
	promptBuilder.WriteString("You are GolfRulesGPT, an expert on the Rules of Golf. ")
	promptBuilder.WriteString("Answer questions about golf rules accurately based on the provided context. ")
	promptBuilder.WriteString("When referencing rules, use the exact rule numbers from the context. ")
	promptBuilder.WriteString("If the answer is not in the context, say 'I don't have enough information to answer that question based on the official golf rules.'\n\n")

	// Add context with hierarchical information
	promptBuilder.WriteString("Context from the Official Rules of Golf:\n")
	for i, ctx := range contexts {
		// Enhance context with hierarchical information
		pageNum := ctx.Metadata.PageNumber
		hierarchy := ctx.Metadata.Hierarchy
		subsection := ctx.Metadata.Subsection

		contextHeader := fmt.Sprintf("Context %d [%s", i+1, hierarchy)
		if subsection != "" {
			contextHeader += fmt.Sprintf(" > %s", subsection)
		}
		contextHeader += fmt.Sprintf(", Page: %d]:\n", pageNum)

		promptBuilder.WriteString(contextHeader)
		promptBuilder.WriteString(ctx.Content)
		promptBuilder.WriteString("\n\n")
	}

	// Add query
	promptBuilder.WriteString("Question: " + query + "\n\n")
	promptBuilder.WriteString("Answer: ")

	return promptBuilder.String()
}

// GenerateResponse generates a response from the LLM
func (o *OllamaLLM) GenerateResponse(ctx context.Context, prompt string) (string, error) {
	req := api.GenerateRequest{
		Model:  o.Model,
		Prompt: prompt,
		Options: map[string]interface{}{
			"temperature": 0.1,
			"num_predict": 1024,
		},
	}

	var responseBuilder strings.Builder

	err := o.Client.Generate(ctx, &req, func(resp api.GenerateResponse) error {
		_, err := responseBuilder.WriteString(resp.Response)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate response: %w", err)
	}

	return responseBuilder.String(), nil
}

// Answer answers a query using the LLM and context
func (o *OllamaLLM) Answer(ctx context.Context, query string, contexts []models.TextChunk) (*models.Response, error) {
	prompt := o.GeneratePrompt(query, contexts)

	answer, err := o.GenerateResponse(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	timestamp := time.Now().Format(time.RFC3339)

	return &models.Response{
		Answer:    answer,
		Sources:   contexts,
		Timestamp: timestamp,
	}, nil
}

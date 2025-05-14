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

// GeneratePrompt creates a prompt for the LLM with enhanced structural context
func (o *OllamaLLM) GeneratePrompt(query string, contexts []models.TextChunk) string {
	var promptBuilder strings.Builder

	// Enhanced system instruction
	promptBuilder.WriteString("You are GolfRulesGPT, an expert on the Official Rules of Golf. ")
	promptBuilder.WriteString("Answer questions about golf rules accurately based on the provided context. ")
	promptBuilder.WriteString("When referencing rules, use the exact rule numbers and include complete hierarchical references (e.g., Rule 11.2b(1)). ")
	promptBuilder.WriteString("If you need to reference a definition, use its proper name from the Rules of Golf. ")
	promptBuilder.WriteString("If the answer is not in the context, say 'I don't have enough information to answer that question based on the official golf rules.'\n\n")

	// Add context with full hierarchical information
	promptBuilder.WriteString("Context from the Official Rules of Golf:\n")
	for i, ctx := range contexts {
		// Include detailed hierarchical information
		hierarchyPath := ctx.Metadata.Hierarchy
		chunkType := ctx.Metadata.ChunkType

		contextHeader := fmt.Sprintf("Context %d [Type: %s, Path: %s", i+1, chunkType, hierarchyPath)
		if ctx.Metadata.Subsection != "" {
			if ctx.Metadata.SubsecTitle != "" {
				contextHeader += fmt.Sprintf(", Subsection: %s - %s",
					ctx.Metadata.Subsection, ctx.Metadata.SubsecTitle)
			} else {
				contextHeader += fmt.Sprintf(", Subsection: %s", ctx.Metadata.Subsection)
			}
		}
		contextHeader += fmt.Sprintf(", Page: %d]:\n", ctx.Metadata.PageNumber)

		promptBuilder.WriteString(contextHeader)
		promptBuilder.WriteString(ctx.Content)

		// Add cross-references if available
		if len(ctx.CrossReferences) > 0 {
			promptBuilder.WriteString("\nCross References: ")
			promptBuilder.WriteString(strings.Join(ctx.CrossReferences, ", "))
		}

		// Add index terms if available
		if len(ctx.IndexTerms) > 0 {
			promptBuilder.WriteString("\nKey Terms: ")
			promptBuilder.WriteString(strings.Join(ctx.IndexTerms, ", "))
		}

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

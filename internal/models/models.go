package models

// TextChunk represents a chunk of text from the PDF
type TextChunk struct {
	ID        int       `json:"id"`
	Content   string    `json:"content"`
	Metadata  Metadata  `json:"metadata"`
	Embedding []float64 `json:"embedding"`
}

// Metadata contains information about the text chunk
type Metadata struct {
	PageNumber int    `json:"page_number"`
	Section    string `json:"section"`
	Title      string `json:"title"`
	Hierarchy  string `json:"hierarchy"`
	Subsection string `json:"subsection,omitempty"`
}

// RuleHierarchy represents the hierarchical structure of golf rules
type RuleHierarchy struct {
	RuleNumber string             `json:"rule_number"`
	Title      string             `json:"title"`
	PageNumber int                `json:"page_number"`
	Subrules   map[string]SubRule `json:"subrules"`
	Path       string             `json:"path"`
}

// SubRule represents a subrule within a rule
type SubRule struct {
	Number     string            `json:"number"`
	Title      string            `json:"title"`
	PageNumber int               `json:"page_number"`
	Exceptions map[string]string `json:"exceptions"`
	Path       string            `json:"path"`
}

// Query represents a user query
type Query struct {
	Text      string    `json:"text"`
	Embedding []float64 `json:"embedding"`
}

// Response represents the response from the LLM
type Response struct {
	Answer    string      `json:"answer"`
	Sources   []TextChunk `json:"sources"`
	Timestamp string      `json:"timestamp"`
}

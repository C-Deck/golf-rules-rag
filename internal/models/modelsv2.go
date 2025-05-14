//go:build v2

package models

// TextChunk represents a chunk of text from the PDF
type TextChunk struct {
	ID              int       `json:"id"`
	Content         string    `json:"content"`
	Metadata        Metadata  `json:"metadata"`
	Embedding       []float64 `json:"embedding"`
	CrossReferences []string  `json:"cross_references,omitempty"`
	IndexTerms      []string  `json:"index_terms,omitempty"`
}

// Metadata contains information about the text chunk
type Metadata struct {
	PageNumber  int    `json:"page_number"`
	Section     string `json:"section"`                // Rule number (e.g., "Rule 13")
	Title       string `json:"title"`                  // Rule title (e.g., "Putting Greens")
	Hierarchy   string `json:"hierarchy"`              // Complete path (e.g., "Rule 13 > 13.1 > 13.1c")
	Subsection  string `json:"subsection,omitempty"`   // Subsection number (e.g., "13.1c")
	SubsecTitle string `json:"subsec_title,omitempty"` // Subsection title
	ChunkType   string `json:"chunk_type,omitempty"`   // "rule", "definition", "index", etc.
	ParentRule  string `json:"parent_rule,omitempty"`  // For subsections
}

// GolfRuleHierarchy represents the hierarchical structure of golf rules
type GolfRuleHierarchy struct {
	RuleNumber string                 `json:"rule_number"`
	Title      string                 `json:"title"`
	PageNumber int                    `json:"page_number"`
	Sections   map[string]RuleSection `json:"sections"`
	Path       string                 `json:"path"`
	IndexTerms []string               `json:"index_terms,omitempty"`
}

// RuleSection represents a section within a rule
type RuleSection struct {
	Number          string                    `json:"number"`
	Title           string                    `json:"title"`
	PageNumber      int                       `json:"page_number"`
	Subsections     map[string]RuleSubsection `json:"subsections,omitempty"`
	Content         string                    `json:"content,omitempty"`
	Path            string                    `json:"path"`
	CrossReferences []string                  `json:"cross_references,omitempty"`
}

// RuleSubsection represents a subsection within a section
type RuleSubsection struct {
	Number          string   `json:"number"`
	Title           string   `json:"title"`
	Content         string   `json:"content"`
	PageNumber      int      `json:"page_number"`
	Path            string   `json:"path"`
	CrossReferences []string `json:"cross_references,omitempty"`
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

// IndexEntry represents an entry in the rules index
type IndexEntry struct {
	Term           string   `json:"term"`
	RuleReferences []string `json:"rule_references"`
}

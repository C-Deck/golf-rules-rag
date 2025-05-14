// internal/processor/pdf.go
package processor

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"golf-rules-rag/internal/models"

	"github.com/ledongthuc/pdf"
)

const (
	// Maximum size for a single chunk
	MAX_CHUNK_SIZE = 2000
	// Minimum size for a chunk
	MIN_CHUNK_SIZE = 100
)

// PDFProcessor handles PDF processing
type PDFProcessor struct {
	ChunkSize    int
	ChunkOverlap int
}

// NewPDFProcessor creates a new PDF processor
func NewPDFProcessor(chunkSize, chunkOverlap int) *PDFProcessor {
	return &PDFProcessor{
		ChunkSize:    chunkSize,
		ChunkOverlap: chunkOverlap,
	}
}

// ExtractText extracts text from a PDF file
func (p *PDFProcessor) ExtractText(filePath string) (string, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("failed to extract plain text: %w", err)
	}

	_, err = buf.ReadFrom(b)
	if err != nil {
		return "", fmt.Errorf("failed to read text: %w", err)
	}

	return buf.String(), nil
}

// ProcessPDF processes a PDF file and returns chunks
func (p *PDFProcessor) ProcessPDF(ctx context.Context, filePath string) ([]models.TextChunk, error) {
	text, err := p.ExtractText(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text: %w", err)
	}

	// Preprocess text
	text = p.preprocessText(text)

	// Extract hierarchical structure and their page numbers
	ruleHierarchy := p.extractHierarchy(text)

	// Create semantic chunks based on the hierarchy
	chunks := p.createSemanticChunks(text, ruleHierarchy)

	return chunks, nil
}

// preprocessText preprocesses the extracted text
func (p *PDFProcessor) preprocessText(text string) string {
	// Remove headers and footers
	text = p.removeHeadersFooters(text)

	// Normalize whitespace
	text = p.normalizeWhitespace(text)

	// Expand golf abbreviations
	text = p.expandGolfAbbreviations(text)

	// Normalize rule references
	text = p.normalizeRuleReferences(text)

	return text
}

// removeHeadersFooters removes headers and footers from the text
func (p *PDFProcessor) removeHeadersFooters(text string) string {
	// Split text by page breaks
	pageRe := regexp.MustCompile(`\f`)
	pages := pageRe.Split(text, -1)

	var cleanedPages []string

	for _, page := range pages {
		lines := strings.Split(page, "\n")

		// Skip empty pages
		if len(lines) < 3 {
			continue
		}

		// Remove header (first 1-2 lines) if it looks like a header
		headerEnd := 0
		for i := 0; i < min(2, len(lines)); i++ {
			if len(strings.TrimSpace(lines[i])) < 50 && (strings.Contains(lines[i], "Rules of Golf") || strings.Contains(lines[i], "Page")) {
				headerEnd = i + 1
			}
		}

		// Remove footer (last 1-2 lines) if it looks like a footer
		footerStart := len(lines)
		for i := len(lines) - 1; i >= max(0, len(lines)-3); i-- {
			if len(strings.TrimSpace(lines[i])) < 50 && (strings.Contains(lines[i], "©") || strings.Contains(lines[i], "Page")) {
				footerStart = i
			}
		}

		// Extract the content between header and footer
		if headerEnd < footerStart {
			cleanedPages = append(cleanedPages, strings.Join(lines[headerEnd:footerStart], "\n"))
		} else {
			cleanedPages = append(cleanedPages, page) // Fallback if detection fails
		}
	}

	return strings.Join(cleanedPages, "\n")
}

// normalizeWhitespace normalizes whitespace in the text
func (p *PDFProcessor) normalizeWhitespace(text string) string {
	// Replace multiple spaces with a single space
	spaceRe := regexp.MustCompile(`\s+`)
	text = spaceRe.ReplaceAllString(text, " ")

	// Ensure paragraphs are separated by double newlines
	paraSepRe := regexp.MustCompile(`\n\s*\n+`)
	text = paraSepRe.ReplaceAllString(text, "\n\n")

	return strings.TrimSpace(text)
}

// expandGolfAbbreviations expands common golf abbreviations
func (p *PDFProcessor) expandGolfAbbreviations(text string) string {
	abbreviations := map[string]string{
		" R&A ":  " Royal and Ancient ",
		" USGA ": " United States Golf Association ",
		" OB ":   " out of bounds ",
		" GUR ":  " ground under repair ",
		"TIO":    "temporary immovable obstruction",
		"DQ":     "disqualification",
	}

	for abbr, expanded := range abbreviations {
		text = strings.ReplaceAll(text, abbr, expanded)
	}

	return text
}

// normalizeRuleReferences normalizes rule references in the text
func (p *PDFProcessor) normalizeRuleReferences(text string) string {
	// Normalize rule references like "Rule 14.3" to a standard format
	ruleRefRe := regexp.MustCompile(`Rule\s+(\d+)([a-z])?(\.\d+)?([a-z])?`)
	text = ruleRefRe.ReplaceAllString(text, "Rule $1$2$3$4")

	return text
}

// extractHierarchy extracts hierarchical structure from the text
func (p *PDFProcessor) extractHierarchy(text string) map[string]models.RuleHierarchy {
	hierarchy := make(map[string]models.RuleHierarchy)

	// Regex for main rule headers
	mainRuleRe := regexp.MustCompile(`(?m)^(Rule \d+)\s*[-–—]\s*(.+?)$`)

	// Regex for subrule headers
	subruleRe := regexp.MustCompile(`(?m)^(\d+\.\d+)\s+(.+?)$`)

	// Regex for exceptions
	exceptionRe := regexp.MustCompile(`(?m)^(Exception|Exception \d+):\s*(.+?)$`)

	// Split text by page breaks to track page numbers
	pageRe := regexp.MustCompile(`\f`)
	pages := pageRe.Split(text, -1)

	currentPageNum := 1
	currentRule := ""

	for _, page := range pages {
		// Find main rules on this page
		mainRuleMatches := mainRuleRe.FindAllStringSubmatch(page, -1)
		for _, match := range mainRuleMatches {
			if len(match) >= 3 {
				ruleNum := match[1]
				ruleTitle := match[2]

				hierarchy[ruleNum] = models.RuleHierarchy{
					RuleNumber: ruleNum,
					Title:      ruleTitle,
					PageNumber: currentPageNum,
					Subrules:   make(map[string]models.SubRule),
					Path:       ruleNum,
				}

				currentRule = ruleNum
			}
		}

		// Find subrules if we're within a rule
		if currentRule != "" {
			subruleMatches := subruleRe.FindAllStringSubmatch(page, -1)
			for _, match := range subruleMatches {
				if len(match) >= 3 {
					subruleNum := match[1]
					subruleTitle := match[2]

					if rule, exists := hierarchy[currentRule]; exists {
						rule.Subrules[subruleNum] = models.SubRule{
							Number:     subruleNum,
							Title:      subruleTitle,
							PageNumber: currentPageNum,
							Exceptions: make(map[string]string),
							Path:       fmt.Sprintf("%s > %s", currentRule, subruleNum),
						}
						hierarchy[currentRule] = rule
					}
				}
			}
		}

		// Find exceptions
		exceptionMatches := exceptionRe.FindAllStringSubmatch(page, -1)
		for _, match := range exceptionMatches {
			if len(match) >= 3 {
				exceptionNum := match[1]
				exceptionText := match[2]

				// Try to associate with the current subrule or rule
				if currentRule != "" {
					if rule, exists := hierarchy[currentRule]; exists {
						// Find the last subrule mentioned
						var lastSubrule string
						for subruleNum := range rule.Subrules {
							if lastSubrule == "" || subruleNum > lastSubrule {
								lastSubrule = subruleNum
							}
						}

						if lastSubrule != "" {
							subrule := rule.Subrules[lastSubrule]
							subrule.Exceptions[exceptionNum] = exceptionText
							rule.Subrules[lastSubrule] = subrule
							hierarchy[currentRule] = rule
						}
					}
				}
			}
		}

		currentPageNum++
	}

	return hierarchy
}

// createSemanticChunks creates chunks based on semantic boundaries
func (p *PDFProcessor) createSemanticChunks(text string, hierarchy map[string]models.RuleHierarchy) []models.TextChunk {
	var chunks []models.TextChunk
	chunkID := 1

	// Split text by main rules
	mainRuleSplitter := regexp.MustCompile(`(?m)^(Rule \d+\s*[-–—]\s*.+?)$`)
	ruleSections := mainRuleSplitter.Split(text, -1)
	ruleHeaders := mainRuleSplitter.FindAllString(text, -1)

	// Process each rule section
	for i, section := range ruleSections {
		if i == 0 && len(strings.TrimSpace(section)) < MIN_CHUNK_SIZE {
			// Skip the text before the first rule if it's too small
			continue
		}

		var ruleNumber, ruleTitle string
		var pageNumber int
		var hierarchyPath string

		// Get the rule information
		if i > 0 && i-1 < len(ruleHeaders) {
			ruleHeader := ruleHeaders[i-1]
			parts := strings.SplitN(ruleHeader, "-", 2)

			if len(parts) >= 1 {
				ruleNumber = strings.TrimSpace(parts[0])

				if rule, exists := hierarchy[ruleNumber]; exists {
					ruleTitle = rule.Title
					pageNumber = rule.PageNumber
					hierarchyPath = rule.Path
				}

				if len(parts) >= 2 {
					ruleTitle = strings.TrimSpace(parts[1])
				}
			}
		}

		// Skip empty sections
		if len(strings.TrimSpace(section)) < MIN_CHUNK_SIZE {
			continue
		}

		// Create chunks based on content size
		if len(section) > MAX_CHUNK_SIZE {
			// Split into semantic subsections (paragraphs)
			paragraphs := strings.Split(section, "\n\n")

			var currentChunk strings.Builder
			var currentSubsection string

			for _, para := range paragraphs {
				// Check if this paragraph is a subsection header
				if len(para) < 100 && (strings.HasPrefix(para, ruleNumber+".") || strings.HasPrefix(para, "Exception")) {
					currentSubsection = strings.TrimSpace(para)
				}

				// If adding this paragraph would exceed the max size, create a new chunk
				if currentChunk.Len()+len(para) > MAX_CHUNK_SIZE && currentChunk.Len() > MIN_CHUNK_SIZE {
					// Create a chunk with the current content
					chunkContent := currentChunk.String()

					chunks = append(chunks, models.TextChunk{
						ID:      chunkID,
						Content: chunkContent,
						Metadata: models.Metadata{
							PageNumber: pageNumber,
							Section:    ruleNumber,
							Title:      ruleTitle,
							Hierarchy:  hierarchyPath,
							Subsection: currentSubsection,
						},
					})
					chunkID++

					// Reset the builder
					currentChunk.Reset()
				}

				// Add the paragraph to the current chunk
				if currentChunk.Len() > 0 {
					currentChunk.WriteString("\n\n")
				}
				currentChunk.WriteString(para)
			}

			// Add any remaining content as a final chunk
			if currentChunk.Len() > 0 {
				chunks = append(chunks, models.TextChunk{
					ID:      chunkID,
					Content: currentChunk.String(),
					Metadata: models.Metadata{
						PageNumber: pageNumber,
						Section:    ruleNumber,
						Title:      ruleTitle,
						Hierarchy:  hierarchyPath,
						Subsection: currentSubsection,
					},
				})
				chunkID++
			}
		} else {
			// This section is small enough to be a single chunk
			chunks = append(chunks, models.TextChunk{
				ID:      chunkID,
				Content: section,
				Metadata: models.Metadata{
					PageNumber: pageNumber,
					Section:    ruleNumber,
					Title:      ruleTitle,
					Hierarchy:  hierarchyPath,
				},
			})
			chunkID++
		}
	}

	return chunks
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

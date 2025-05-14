//go:build v2

package processor

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	_ "unicode"

	"golf-rules-rag/internal/models"

	"github.com/ledongthuc/pdf"
)

const (
	// Constants for chunk sizes and overlap
	MAX_CHUNK_SIZE  = 2000
	MIN_CHUNK_SIZE  = 100
	DEFAULT_OVERLAP = 200

	// Document sections
	SECTION_RULES       = "RULES"
	SECTION_DEFINITIONS = "DEFINITIONS"
	SECTION_INDEX       = "INDEX"
)

// PDFProcessor handles PDF processing
type PDFProcessor struct {
	ChunkSize    int
	ChunkOverlap int
}

// NewPDFProcessor creates a new PDF processor
func NewPDFProcessor(chunkSize, chunkOverlap int) *PDFProcessor {
	if chunkSize <= 0 {
		chunkSize = MAX_CHUNK_SIZE
	}
	if chunkOverlap <= 0 {
		chunkOverlap = DEFAULT_OVERLAP
	}

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

// ProcessPDF processes a PDF file and returns optimized chunks for golf rules
func (p *PDFProcessor) ProcessPDF(ctx context.Context, filePath string) ([]models.TextChunk, error) {
	text, err := p.ExtractText(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text: %w", err)
	}

	// Preprocess text for golf-specific content
	text = p.preprocessGolfRules(text)

	// Extract different document sections
	ruleText, definitionsText, indexText := p.extractDocumentSections(text)

	// Process rules
	ruleHierarchy := p.extractRulesHierarchy(ruleText)

	// Process definitions
	definitionChunks := p.processDefinitions(definitionsText)

	// Process index
	indexEntries := p.processIndex(indexText)

	// Apply index terms to rule hierarchy
	p.applyIndexTermsToRules(ruleHierarchy, indexEntries)

	// Create optimized chunks based on the rule hierarchy
	chunks := p.createRuleBasedChunks(ruleHierarchy)

	// Add definition chunks
	chunks = append(chunks, definitionChunks...)

	// Extract cross-references and update chunks
	p.extractCrossReferences(chunks)

	return chunks, nil
}

// preprocessGolfRules applies golf-specific preprocessing to the text
func (p *PDFProcessor) preprocessGolfRules(text string) string {
	// Remove headers and footers
	text = p.removeHeadersFooters(text)

	// Normalize whitespace
	text = p.normalizeWhitespace(text)

	// Normalize rule references
	text = p.normalizeRuleReferences(text)

	// Expand golf abbreviations
	text = p.expandGolfAbbreviations(text)

	// Handle diagram references
	text = p.handleDiagramReferences(text)

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
		for i := 0; i < min(3, len(lines)); i++ {
			line := strings.TrimSpace(lines[i])
			if len(line) < 50 && (strings.Contains(line, "Rules of Golf") ||
				strings.Contains(line, "Page") || strings.Contains(line, "Contents")) {
				headerEnd = i + 1
			}
		}

		// Remove footer (last 1-3 lines) if it looks like a footer
		footerStart := len(lines)
		for i := len(lines) - 1; i >= max(0, len(lines)-4); i-- {
			line := strings.TrimSpace(lines[i])
			if len(line) < 50 && (strings.Contains(line, "©") ||
				strings.Contains(line, "Page") || strings.Contains(line, "R&A") ||
				strings.Contains(line, "USGA")) {
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

// normalizeRuleReferences standardizes rule references throughout the text
func (p *PDFProcessor) normalizeRuleReferences(text string) string {
	// Normalize rule references like "Rule 14.3" to a standard format
	ruleRefRe := regexp.MustCompile(`Rule\s+(\d+)([a-z])?(\.\d+)?([a-z])?`)
	text = ruleRefRe.ReplaceAllString(text, "Rule $1$2$3$4")

	// Fix common OCR errors in rule numbers
	text = strings.ReplaceAll(text, "Ru1e", "Rule")
	text = strings.ReplaceAll(text, "Ruie", "Rule")

	return text
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

// handleDiagramReferences preserves diagram references
func (p *PDFProcessor) handleDiagramReferences(text string) string {
	// Identify and mark diagram references for preservation
	diagramRe := regexp.MustCompile(`DIAGRAM\s+(\d+(\.\d+)?[a-z]?)`)
	text = diagramRe.ReplaceAllString(text, "[DIAGRAM_REF:$1]")

	return text
}

// extractDocumentSections separates the PDF into rules, definitions, and index sections
func (p *PDFProcessor) extractDocumentSections(text string) (string, string, string) {
	// Find the definitions section (starts with "Definitions")
	definitionsStartRe := regexp.MustCompile(`(?i)XI\.\s+Definitions`)
	definitionsMatches := definitionsStartRe.FindStringIndex(text)

	// Find the index section (starts with "Index")
	indexStartRe := regexp.MustCompile(`(?i)Index\s*\n`)
	indexMatches := indexStartRe.FindStringIndex(text)

	var ruleText, definitionsText, indexText string

	if len(definitionsMatches) > 0 && len(indexMatches) > 0 {
		// We found both sections
		ruleText = text[:definitionsMatches[0]]
		definitionsText = text[definitionsMatches[0]:indexMatches[0]]
		indexText = text[indexMatches[0]:]
	} else if len(definitionsMatches) > 0 {
		// Only found definitions
		ruleText = text[:definitionsMatches[0]]
		definitionsText = text[definitionsMatches[0]:]
	} else if len(indexMatches) > 0 {
		// Only found index
		ruleText = text[:indexMatches[0]]
		indexText = text[indexMatches[0]:]
	} else {
		// Found neither, assume it's all rules
		ruleText = text
	}

	return ruleText, definitionsText, indexText
}

// extractRulesHierarchy builds the complete rule hierarchy
func (p *PDFProcessor) extractRulesHierarchy(text string) map[string]models.GolfRuleHierarchy {
	hierarchy := make(map[string]models.GolfRuleHierarchy)

	// Patterns for rules hierarchy
	mainRuleRe := regexp.MustCompile(`(?m)^(Rule\s+\d+)\s*[–—-]\s*(.+?)$`)
	sectionRe := regexp.MustCompile(`(?m)^(\d+\.\d+)\s+(.+?)$`)
	subsectionRe := regexp.MustCompile(`(?m)^(\d+\.\d+[a-z](?:\(\d+\))?)\s+(.+?)$`)

	// Split text by page breaks to track page numbers
	pageRe := regexp.MustCompile(`\f`)
	pages := pageRe.Split(text, -1)

	currentPageNum := 1
	currentRule := ""
	currentSection := ""

	// Process each page
	for _, page := range pages {
		// Find main rules on this page
		mainRuleMatches := mainRuleRe.FindAllStringSubmatchIndex(page, -1)

		for i, match := range mainRuleMatches {
			ruleStart := match[0]
			ruleEnd := len(page)
			if i < len(mainRuleMatches)-1 {
				ruleEnd = mainRuleMatches[i+1][0]
			}

			ruleText := page[ruleStart:ruleEnd]
			ruleNum := strings.TrimSpace(page[match[2]:match[3]])
			ruleTitle := strings.TrimSpace(page[match[4]:match[5]])

			// Create rule entry
			hierarchy[ruleNum] = models.GolfRuleHierarchy{
				RuleNumber: ruleNum,
				Title:      ruleTitle,
				PageNumber: currentPageNum,
				Sections:   make(map[string]models.RuleSection),
				Path:       ruleNum,
			}

			currentRule = ruleNum

			// Find sections within this rule
			sectionMatches := sectionRe.FindAllStringSubmatchIndex(ruleText, -1)

			for j, sectionMatch := range sectionMatches {
				sectionStart := sectionMatch[0]
				sectionEnd := len(ruleText)
				if j < len(sectionMatches)-1 {
					sectionEnd = sectionMatches[j+1][0]
				}

				sectionText := ruleText[sectionStart:sectionEnd]
				sectionNum := strings.TrimSpace(ruleText[sectionMatch[2]:sectionMatch[3]])
				sectionTitle := strings.TrimSpace(ruleText[sectionMatch[4]:sectionMatch[5]])

				// Check if this rule exists in our hierarchy
				if rule, exists := hierarchy[currentRule]; exists {
					rule.Sections[sectionNum] = models.RuleSection{
						Number:      sectionNum,
						Title:       sectionTitle,
						PageNumber:  currentPageNum,
						Subsections: make(map[string]models.RuleSubsection),
						Content:     sectionText,
						Path:        fmt.Sprintf("%s > %s", currentRule, sectionNum),
					}
					hierarchy[currentRule] = rule

					currentSection = sectionNum

					// Find subsections within this section
					subsectionMatches := subsectionRe.FindAllStringSubmatchIndex(sectionText, -1)

					for k, subsectionMatch := range subsectionMatches {
						subsectionStart := subsectionMatch[0]
						subsectionEnd := len(sectionText)
						if k < len(subsectionMatches)-1 {
							subsectionEnd = subsectionMatches[k+1][0]
						}

						subsectionText := sectionText[subsectionStart:subsectionEnd]
						subsectionNum := strings.TrimSpace(sectionText[subsectionMatch[2]:subsectionMatch[3]])
						subsectionTitle := strings.TrimSpace(sectionText[subsectionMatch[4]:subsectionMatch[5]])

						// Check if this section exists in our rule
						if section, exists := rule.Sections[currentSection]; exists {
							section.Subsections[subsectionNum] = models.RuleSubsection{
								Number:     subsectionNum,
								Title:      subsectionTitle,
								Content:    subsectionText,
								PageNumber: currentPageNum,
								Path:       fmt.Sprintf("%s > %s > %s", currentRule, currentSection, subsectionNum),
							}
							rule.Sections[currentSection] = section
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

// processDefinitions extracts and chunks the definitions section
func (p *PDFProcessor) processDefinitions(text string) []models.TextChunk {
	if text == "" {
		return nil
	}

	var chunks []models.TextChunk
	chunkID := 1

	// Pattern to match individual definitions
	defRe := regexp.MustCompile(`(?m)^([A-Z][A-Za-z\s-]+)\n`)

	// Find all definitions
	defMatches := defRe.FindAllStringSubmatchIndex(text, -1)

	for i, match := range defMatches {
		defStart := match[0]
		defEnd := len(text)
		if i < len(defMatches)-1 {
			defEnd = defMatches[i+1][0]
		}

		defText := text[defStart:defEnd]
		defTerm := strings.TrimSpace(text[match[2]:match[3]])

		// Create a chunk for this definition
		chunks = append(chunks, models.TextChunk{
			ID:      chunkID,
			Content: defText,
			Metadata: models.Metadata{
				Section:   "Definitions",
				Title:     defTerm,
				ChunkType: "definition",
				Hierarchy: fmt.Sprintf("Definitions > %s", defTerm),
			},
		})

		chunkID++
	}

	return chunks
}

// processIndex extracts index entries
func (p *PDFProcessor) processIndex(text string) []models.IndexEntry {
	if text == "" {
		return nil
	}

	var entries []models.IndexEntry

	// Pattern to match index entries
	indexRe := regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z\s\-,]+)(\s+\d+(?:[-,]\d+)*)$`)

	// Find all index entries
	indexMatches := indexRe.FindAllStringSubmatch(text, -1)

	for _, match := range indexMatches {
		if len(match) >= 3 {
			term := strings.TrimSpace(match[1])
			references := strings.TrimSpace(match[2])

			// Extract rule references
			var ruleRefs []string
			refNumbersRe := regexp.MustCompile(`\d+`)
			ruleNumbers := refNumbersRe.FindAllString(references, -1)

			for _, num := range ruleNumbers {
				ruleRefs = append(ruleRefs, fmt.Sprintf("Rule %s", num))
			}

			entries = append(entries, models.IndexEntry{
				Term:           term,
				RuleReferences: ruleRefs,
			})
		}
	}

	return entries
}

// applyIndexTermsToRules associates index terms with their relevant rules
func (p *PDFProcessor) applyIndexTermsToRules(ruleHierarchy map[string]models.GolfRuleHierarchy,
	indexEntries []models.IndexEntry) {

	// Build a map of rule references to index terms
	ruleTerms := make(map[string][]string)

	for _, entry := range indexEntries {
		for _, ruleRef := range entry.RuleReferences {
			ruleTerms[ruleRef] = append(ruleTerms[ruleRef], entry.Term)
		}
	}

	// Associate terms with rules
	for ruleNum, rule := range ruleHierarchy {
		if terms, exists := ruleTerms[ruleNum]; exists {
			rule.IndexTerms = terms
			ruleHierarchy[ruleNum] = rule
		}
	}
}

// createRuleBasedChunks converts the rule hierarchy into optimized chunks
func (p *PDFProcessor) createRuleBasedChunks(ruleHierarchy map[string]models.GolfRuleHierarchy) []models.TextChunk {
	var chunks []models.TextChunk
	chunkID := 1

	// For each rule in the hierarchy
	for ruleNum, rule := range ruleHierarchy {
		// Create a chunk for the main rule
		ruleIntro := fmt.Sprintf("%s – %s\n", ruleNum, rule.Title)
		chunks = append(chunks, models.TextChunk{
			ID:      chunkID,
			Content: ruleIntro,
			Metadata: models.Metadata{
				PageNumber: rule.PageNumber,
				Section:    ruleNum,
				Title:      rule.Title,
				Hierarchy:  rule.Path,
				ChunkType:  "rule",
			},
			IndexTerms: rule.IndexTerms,
		})
		chunkID++

		// For each section in the rule
		for sectionNum, section := range rule.Sections {
			// Check if section content is large enough to be split
			if len(section.Content) > p.ChunkSize {
				// Split large sections into multiple chunks
				chunks = append(chunks, p.splitSectionIntoChunks(
					section.Content,
					chunkID,
					ruleNum,
					rule.Title,
					sectionNum,
					section.Title,
					section.Path,
					section.PageNumber,
					rule.IndexTerms)...)

				chunkID += len(chunks)
			} else {
				// Add section as a single chunk
				chunks = append(chunks, models.TextChunk{
					ID:      chunkID,
					Content: section.Content,
					Metadata: models.Metadata{
						PageNumber:  section.PageNumber,
						Section:     ruleNum,
						Title:       rule.Title,
						Subsection:  sectionNum,
						SubsecTitle: section.Title,
						Hierarchy:   section.Path,
						ParentRule:  ruleNum,
						ChunkType:   "section",
					},
					IndexTerms: rule.IndexTerms,
				})
				chunkID++
			}

			// Add subsections separately for better retrieval
			for subsectionNum, subsection := range section.Subsections {
				chunks = append(chunks, models.TextChunk{
					ID:      chunkID,
					Content: subsection.Content,
					Metadata: models.Metadata{
						PageNumber:  subsection.PageNumber,
						Section:     ruleNum,
						Title:       rule.Title,
						Subsection:  subsectionNum,
						SubsecTitle: subsection.Title,
						Hierarchy:   subsection.Path,
						ParentRule:  ruleNum,
						ChunkType:   "subsection",
					},
					IndexTerms: rule.IndexTerms,
				})
				chunkID++
			}
		}
	}

	return chunks
}

// splitSectionIntoChunks splits a large section into multiple chunks
func (p *PDFProcessor) splitSectionIntoChunks(content string, startID int,
	ruleNum, ruleTitle, sectionNum, sectionTitle, path string, pageNum int,
	indexTerms []string) []models.TextChunk {

	var chunks []models.TextChunk
	chunkID := startID

	// Split into paragraphs
	paragraphs := strings.Split(content, "\n\n")

	var currentChunk strings.Builder
	for _, para := range paragraphs {
		// If adding this paragraph would make the chunk too large
		if currentChunk.Len()+len(para) > p.ChunkSize && currentChunk.Len() > MIN_CHUNK_SIZE {
			// Create a chunk with current content
			chunks = append(chunks, models.TextChunk{
				ID:      chunkID,
				Content: currentChunk.String(),
				Metadata: models.Metadata{
					PageNumber:  pageNum,
					Section:     ruleNum,
					Title:       ruleTitle,
					Subsection:  sectionNum,
					SubsecTitle: sectionTitle,
					Hierarchy:   path,
					ParentRule:  ruleNum,
					ChunkType:   "section",
				},
				IndexTerms: indexTerms,
			})
			chunkID++

			// Reset the builder with overlap
			currentChunk = strings.Builder{}

			// Include the last paragraph for overlap context
			if len(chunks) > 0 && len(paragraphs) > 1 {
				lastPara := getLastParagraph(chunks[len(chunks)-1].Content)
				if len(lastPara) > 0 {
					currentChunk.WriteString(lastPara)
					currentChunk.WriteString("\n\n")
				}
			}
		}

		// Add the paragraph to the current chunk
		if currentChunk.Len() > 0 {
			currentChunk.WriteString("\n\n")
		}
		currentChunk.WriteString(para)
	}

	// Add the final chunk if there's content left
	if currentChunk.Len() > 0 {
		chunks = append(chunks, models.TextChunk{
			ID:      chunkID,
			Content: currentChunk.String(),
			Metadata: models.Metadata{
				PageNumber:  pageNum,
				Section:     ruleNum,
				Title:       ruleTitle,
				Subsection:  sectionNum,
				SubsecTitle: sectionTitle,
				Hierarchy:   path,
				ParentRule:  ruleNum,
				ChunkType:   "section",
			},
			IndexTerms: indexTerms,
		})
	}

	return chunks
}

// extractCrossReferences finds and assigns cross-references to each chunk
func (p *PDFProcessor) extractCrossReferences(chunks []models.TextChunk) {
	ruleRefPattern := regexp.MustCompile(`(Rule \d+(\.\d+[a-z]?)?)`)

	for i, chunk := range chunks {
		// Find all rule references in the chunk
		matches := ruleRefPattern.FindAllString(chunk.Content, -1)

		// Deduplicate references
		refMap := make(map[string]bool)
		for _, match := range matches {
			refMap[match] = true
		}

		// Convert map back to slice
		var refs []string
		for ref := range refMap {
			refs = append(refs, ref)
		}

		// Update the chunk's cross-references
		chunks[i].CrossReferences = refs
	}
}

// getLastParagraph extracts the last paragraph from text
func getLastParagraph(text string) string {
	paragraphs := strings.Split(text, "\n\n")
	if len(paragraphs) > 0 {
		return paragraphs[len(paragraphs)-1]
	}
	return ""
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

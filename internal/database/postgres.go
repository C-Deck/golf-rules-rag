package database

import (
	"context"
	"fmt"
	"regexp"

	"golf-rules-rag/internal/models"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB represents the database connection
type DB struct {
	Pool *pgxpool.Pool
}

// NewDB creates a new database connection
func NewDB(connStr string) (*DB, error) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Initialize sets up the database tables and indices
func (db *DB) Initialize(ctx context.Context) error {
	// Create table for text chunks with vector extension
	_, err := db.Pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS text_chunks (
            id SERIAL PRIMARY KEY,
            content TEXT NOT NULL,
            page_number INTEGER NOT NULL,
            section TEXT,
            title TEXT,
            hierarchy TEXT,
            subsection TEXT,
            subsec_title TEXT,
            chunk_type TEXT,
            parent_rule TEXT,
            cross_references TEXT[],
            index_terms TEXT[],
            embedding vector(384) NOT NULL
        )
    `)
	if err != nil {
		return fmt.Errorf("failed to create text_chunks table: %w", err)
	}

	// Create vector index
	_, err = db.Pool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS text_chunks_embedding_idx ON text_chunks 
		USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)
	`)
	if err != nil {
		return fmt.Errorf("failed to create vector index: %w", err)
	}

	// Create indices for better query performance
	_, err = db.Pool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS text_chunks_section_idx ON text_chunks (section);
		CREATE INDEX IF NOT EXISTS text_chunks_hierarchy_idx ON text_chunks (hierarchy);
	`)
	if err != nil {
		return fmt.Errorf("failed to create additional indices: %w", err)
	}

	return nil
}

// StoreTextChunk stores a text chunk in the database
func (db *DB) StoreTextChunk(ctx context.Context, chunk *models.TextChunk) error {
	_, err := db.Pool.Exec(ctx, `
        INSERT INTO text_chunks (
            content, page_number, section, title, hierarchy, 
            subsection, subsec_title, chunk_type, parent_rule,
            cross_references, index_terms, embedding
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
    `,
		chunk.Content,
		chunk.Metadata.PageNumber,
		chunk.Metadata.Section,
		chunk.Metadata.Title,
		chunk.Metadata.Hierarchy,
		chunk.Metadata.Subsection,
		chunk.Metadata.SubsecTitle,
		chunk.Metadata.ChunkType,
		chunk.Metadata.ParentRule,
		chunk.CrossReferences,
		chunk.IndexTerms,
		chunk.Embedding)

	return err
}

// QueryByRuleNumber finds chunks for a specific rule
func (db *DB) QueryByRuleNumber(ctx context.Context, ruleNumber string) ([]models.TextChunk, error) {
	rows, err := db.Pool.Query(ctx, `
        SELECT id, content, page_number, section, title, hierarchy, 
               subsection, subsec_title, chunk_type, parent_rule,
               cross_references, index_terms
        FROM text_chunks
        WHERE section = $1 OR parent_rule = $1
        ORDER BY hierarchy, subsection
    `, ruleNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to query rule reference chunks: %w", err)
	}
	return processRows(rows)
}

// QueryByRuleReference finds chunks that reference a specific rule
func (db *DB) QueryByRuleReference(ctx context.Context, ruleRef string) ([]models.TextChunk, error) {
	rows, err := db.Pool.Query(ctx, `
        SELECT id, content, page_number, section, title, hierarchy, 
               subsection, subsec_title, chunk_type, parent_rule,
               cross_references, index_terms
        FROM text_chunks
        WHERE $1 = ANY(cross_references)
        ORDER BY hierarchy, subsection
    `, ruleRef)
	if err != nil {
		return nil, fmt.Errorf("failed to query rule reference chunks: %w", err)
	}
	return processRows(rows)
}

// QuerySimilarWithStructure Enhanced query function that leverages both vector similarity and rule structure
func (db *DB) QuerySimilarWithStructure(ctx context.Context, embedding []float64, query string, limit int) ([]models.TextChunk, error) {
	// Extract rule references from the query
	rulePattern := regexp.MustCompile(`Rule\s+(\d+)(\.\d+)?([a-z])?(\(\d+\))?`)
	matches := rulePattern.FindAllStringSubmatch(query, -1)

	var ruleReferences []string
	for _, match := range matches {
		if len(match) > 0 {
			ruleRef := match[0]
			ruleReferences = append(ruleReferences, ruleRef)
		}
	}

	// If rule references found, prioritize those chunks
	if len(ruleReferences) > 0 {
		rows, err := db.Pool.Query(ctx, `
            WITH rule_chunks AS (
                SELECT id, content, page_number, section, title, hierarchy, 
                       subsection, subsec_title, chunk_type, parent_rule,
                       cross_references, index_terms, embedding
                FROM text_chunks
                WHERE section = ANY($1) OR parent_rule = ANY($1) OR 
                      EXISTS (SELECT 1 FROM unnest(cross_references) AS ref 
                              WHERE ref = ANY($1))
            )
            SELECT id, content, page_number, section, title, hierarchy, 
                   subsection, subsec_title, chunk_type, parent_rule,
                   cross_references, index_terms
            FROM rule_chunks
            ORDER BY embedding <=> $2
            LIMIT $3
        `, ruleReferences, embedding, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to query similar structure chunks: %w", err)
		}
		return processRows(rows)
	} else {
		// Fall back to pure vector similarity
		return db.QuerySimilar(ctx, embedding, limit)
	}
}

// QuerySimilar finds chunks similar to the query embedding
func (db *DB) QuerySimilar(ctx context.Context, embedding []float64, limit int) ([]models.TextChunk, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, content, page_number, section, title, hierarchy, 
                       subsection, subsec_title, chunk_type, parent_rule,
                       cross_references, index_terms, embedding
		FROM text_chunks
		ORDER BY embedding <=> $1
		LIMIT $2
	`, embedding, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query similar chunks: %w", err)
	}

	defer rows.Close()

	var chunks []models.TextChunk
	for rows.Next() {
		var (
			chunk                                                                     models.TextChunk
			pageNum                                                                   int
			section, title, hierarchy, subsection, subsecTitle, chunkType, parentRule string
			crossRefs, indexTerms                                                     []string
			embeddingCol                                                              []float64
		)

		if err := rows.Scan(
			&chunk.ID,
			&chunk.Content,
			&pageNum,
			&section,
			&title,
			&hierarchy,
			&subsection,
			&subsecTitle,
			&chunkType,
			&parentRule,
			&crossRefs,
			&indexTerms, &embeddingCol); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		chunk.Metadata = models.Metadata{
			PageNumber:  pageNum,
			Section:     section,
			Title:       title,
			Hierarchy:   hierarchy,
			Subsection:  subsection,
			SubsecTitle: subsecTitle,
			ChunkType:   chunkType,
			ParentRule:  parentRule,
		}
		chunk.CrossReferences = crossRefs
		chunk.IndexTerms = indexTerms

		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return chunks, nil
}

func processRows(rows pgx.Rows) ([]models.TextChunk, error) {
	defer rows.Close()

	var chunks []models.TextChunk
	for rows.Next() {
		var chunk models.TextChunk
		var pageNum int
		var section, title, hierarchy, subsection, subsecTitle, chunkType, parentRule string
		var crossRefs, indexTerms []string

		if err := rows.Scan(
			&chunk.ID,
			&chunk.Content,
			&pageNum,
			&section,
			&title,
			&hierarchy,
			&subsection,
			&subsecTitle,
			&chunkType,
			&parentRule,
			&crossRefs,
			&indexTerms); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		chunk.Metadata = models.Metadata{
			PageNumber:  pageNum,
			Section:     section,
			Title:       title,
			Hierarchy:   hierarchy,
			Subsection:  subsection,
			SubsecTitle: subsecTitle,
			ChunkType:   chunkType,
			ParentRule:  parentRule,
		}
		chunk.CrossReferences = crossRefs
		chunk.IndexTerms = indexTerms

		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return chunks, nil
}

// QuerySimilarWithFilters finds chunks similar to the query embedding with optional filters
func (db *DB) QuerySimilarWithFilters(ctx context.Context, embedding []float64, limit int,
	sectionFilter string) ([]models.TextChunk, error) {

	var rows pgx.Rows
	var err error

	if sectionFilter != "" {
		// Query with section filter
		rows, err = db.Pool.Query(ctx, `
			SELECT id, content, page_number, section, title, hierarchy, 
                       subsection, subsec_title, chunk_type, parent_rule,
                       cross_references, index_terms, embedding
			FROM text_chunks
			WHERE section LIKE $1
			ORDER BY embedding <=> $2
			LIMIT $3
		`, sectionFilter+"%", embedding, limit)
	} else {
		// Query without filter
		rows, err = db.Pool.Query(ctx, `
			SELECT id, content, page_number, section, title, hierarchy, 
                       subsection, subsec_title, chunk_type, parent_rule,
                       cross_references, index_terms, embedding
			FROM text_chunks
			ORDER BY embedding <=> $1
			LIMIT $2
		`, embedding, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query similar chunks: %w", err)
	}
	defer rows.Close()

	var chunks []models.TextChunk
	for rows.Next() {
		var chunk models.TextChunk
		var pageNum int
		var section, title, hierarchy, subsection string

		if err := rows.Scan(&chunk.ID, &chunk.Content, &pageNum, &section, &title, &hierarchy, &subsection); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		chunk.Metadata = models.Metadata{
			PageNumber: pageNum,
			Section:    section,
			Title:      title,
			Hierarchy:  hierarchy,
			Subsection: subsection,
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// QuerySimilarWithTerms enhances vector search with golf-specific term filtering
func (db *DB) QuerySimilarWithTerms(ctx context.Context, embedding []float64, terms []string, limit int) ([]models.TextChunk, error) {
	// Convert terms array to SQL array format
	termParams := make([]interface{}, len(terms)+1)
	termParams[0] = embedding

	// Build the SQL query with dynamic term filtering
	query := `
        WITH term_matches AS (
            SELECT id, content, page_number, section, title, hierarchy, 
                   subsection, subsec_title, chunk_type, parent_rule,
                   cross_references, index_terms, embedding,
                   (
    `

	// Add a score component for each term
	for i, term := range terms {
		if i > 0 {
			query += " + "
		}
		query += fmt.Sprintf("CASE WHEN content ILIKE '%%' || $%d || '%%' THEN 0.5 ELSE 0 END", i+2)
		termParams[i+1] = term
	}

	// Complete the query
	query += `
                   ) AS term_score
            FROM text_chunks
        )
        SELECT id, content, page_number, section, title, hierarchy, 
               subsection, subsec_title, chunk_type, parent_rule,
               cross_references, index_terms
        FROM term_matches
        ORDER BY term_score DESC, embedding <=> $1
        LIMIT $` + fmt.Sprintf("%d", len(termParams)+1)

	termParams = append(termParams, limit)

	rows, err := db.Pool.Query(ctx, query, termParams...)
	if err != nil {
		return nil, fmt.Errorf("failed to query similar chunks with terms: %w", err)
	}
	defer rows.Close()

	var chunks []models.TextChunk
	for rows.Next() {
		var chunk models.TextChunk
		var pageNum int
		var section, title, hierarchy, subsection, subsecTitle, chunkType, parentRule string
		var crossRefs, indexTerms []string

		if err := rows.Scan(
			&chunk.ID,
			&chunk.Content,
			&pageNum,
			&section,
			&title,
			&hierarchy,
			&subsection,
			&subsecTitle,
			&chunkType,
			&parentRule,
			&crossRefs,
			&indexTerms); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		chunk.Metadata = models.Metadata{
			PageNumber:  pageNum,
			Section:     section,
			Title:       title,
			Hierarchy:   hierarchy,
			Subsection:  subsection,
			SubsecTitle: subsecTitle,
			ChunkType:   chunkType,
			ParentRule:  parentRule,
		}
		chunk.CrossReferences = crossRefs
		chunk.IndexTerms = indexTerms

		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return chunks, nil
}

// GetRuleSections retrieves all available rule sections
func (db *DB) GetRuleSections(ctx context.Context) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT section FROM text_chunks WHERE section != '' ORDER BY section
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query rule sections: %w", err)
	}
	defer rows.Close()

	var sections []string
	for rows.Next() {
		var section string
		if err := rows.Scan(&section); err != nil {
			return nil, fmt.Errorf("failed to scan section: %w", err)
		}
		sections = append(sections, section)
	}

	return sections, nil
}

// Close closes the database connection
func (db *DB) Close() {
	db.Pool.Close()
}

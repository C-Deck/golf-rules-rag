package database

import (
	"context"
	"fmt"

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
		INSERT INTO text_chunks (content, page_number, section, title, hierarchy, subsection, embedding)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, chunk.Content, chunk.Metadata.PageNumber, chunk.Metadata.Section,
		chunk.Metadata.Title, chunk.Metadata.Hierarchy, chunk.Metadata.Subsection, chunk.Embedding)

	return err
}

// QuerySimilar finds chunks similar to the query embedding
func (db *DB) QuerySimilar(ctx context.Context, embedding []float64, limit int) ([]models.TextChunk, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, content, page_number, section, title, hierarchy, subsection
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

// QuerySimilarWithFilters finds chunks similar to the query embedding with optional filters
func (db *DB) QuerySimilarWithFilters(ctx context.Context, embedding []float64, limit int,
	sectionFilter string) ([]models.TextChunk, error) {

	var rows pgx.Rows
	var err error

	if sectionFilter != "" {
		// Query with section filter
		rows, err = db.Pool.Query(ctx, `
			SELECT id, content, page_number, section, title, hierarchy, subsection
			FROM text_chunks
			WHERE section LIKE $1
			ORDER BY embedding <=> $2
			LIMIT $3
		`, sectionFilter+"%", embedding, limit)
	} else {
		// Query without filter
		rows, err = db.Pool.Query(ctx, `
			SELECT id, content, page_number, section, title, hierarchy, subsection
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

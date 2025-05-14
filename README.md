# Golf Rules RAG CLI Tool

A command-line tool for answering questions about golf rules using Retrieval-Augmented Generation (RAG). This tool uses a local Ollama model and PostgreSQL with pgvector for vector storage.


## System Architecture

### Requirements
- Go 1.23 or later (for building from source)
- Docker and Docker Compose

### Components:

- **PDF Processor & Embedding Creator**: Separate Go program to parse the PDF, chunk text, create embeddings, and store in PostgreSQL
- **CLI Query Tool**: Go program that takes questions, retrieves relevant context, and generates answers
- **Database**: PostgreSQL with `pgvector` for vector similarity search
- **LLM Service**: Ollama running a lightweight model suitable for CPU

### Model Selection
For your hardware constraints (Mac Mini M4, 16GB RAM, CPU-only), recommend:

Phi-3-mini (3.8B parameters) - best balance of performance and resource usage
Alternatives: `Llama3-8B` or `Mistral-7B` if you need stronger reasoning

## Project structure

```
golf-rules-rag/
├── cmd/
│   ├── indexer/         # PDF processing and embedding creation
│   │   └── main.go
│   └── golfqa/          # CLI Q&A tool
│       └── main.go
├── internal/
│   ├── database/        # Database operations
│   │   └── postgres.go
│   ├── embedding/       # Embedding operations
│   │   └── ollama.go
│   ├── llm/             # LLM operations
│   │   └── ollama.go
│   ├── processor/       # PDF processing
│   │   └── pdf.go
│   └── models/          # Data models
│       └── models.go
├── docker-compose.yml   # Run all services
├── Dockerfile           # Build the tools
└── go.mod
```


## Getting Started

### 1. Start the Services

```bash
# Clone this repository
git clone https://github.com/C-Deck/golf-rules-rag.git
cd golf-rules-rag

# Start PostgreSQL and Ollama
docker-compose up -d
```

### 2. Process the Golf Rules PDF

```bash
# Build the tools
docker build -t golf-rules-tools .

# Run the indexer with your golf rules PDF
docker run --rm -v $(pwd):/data golf-rules-tools indexer -pdf /data/golf-rules.pdf -pg "postgres://golfrag:golfrag@postgres:5432/golfrag?sslmode=disable" -ollama http://ollama:11434
```

Or without Docker:

```bash
go run cmd/indexer/main.go -pdf ./golf-rules.pdf
```

### 3. Query the Golf Rules

```bash
# Run in interactive mode
docker run --rm -it golf-rules-tools golfqa -i -pg "postgres://golfrag:golfrag@postgres:5432/golfrag?sslmode=disable" -ollama http://ollama:11434

# Or ask a single question
docker run --rm golf-rules-tools golfqa -q "What is the penalty for a ball hitting another player?" -pg "postgres://golfrag:golfrag@postgres:5432/golfrag?sslmode=disable" -ollama http://ollama:11434
```

Or without Docker:

```bash
# Interactive mode
go run cmd/golfqa/main.go -i

# Single question
go run cmd/golfqa/main.go -q "What is the penalty for a ball hitting another player?"
```

## Command-Line Options

### Indexer
- `-pdf` - Path to PDF file (required)
- `-pg` - PostgreSQL connection string (default: `postgres://golfrag:golfrag@localhost:5432/golfrag?sslmode=disable`)
- `-ollama` - Ollama host (default: uses `OLLAMA_HOST` env var)
- `-model` - Ollama model for embeddings (default: `phi3-mini`)
- `-chunk-size` - Character size for text chunks (default: 1000)
- `-chunk-overlap` - Character overlap between chunks (default: 200)

### Golf Q&A Tool
- `-pg` - PostgreSQL connection string (default: `postgres://golfrag:golfrag@localhost:5432/golfrag?sslmode=disable`)
- `-ollama` - Ollama host (default: uses `OLLAMA_HOST` env var)
- `-model` - Ollama model for answering (default: `phi3-mini`)
- `-embedding-model` - Ollama model for embeddings (default: `phi3-mini`)
- `-context` - Number of similar contexts to retrieve (default: 5)
- `-i` - Run in interactive mode
- `-q` - Query to answer (non-interactive mode)

## Model Selection

This tool uses Phi-3-mini (3.8B parameters) by default, which provides a good balance of performance and resource usage for CPU-only environments. You can also use:

- **Llama3-8B** for stronger reasoning
- **Mistral-7B** for better text generation

## Docker Network Setup

When running the tools with Docker, make sure the containers can communicate with the PostgreSQL and Ollama services by using the correct network configuration:

```bash
# Create a shared network if not using docker-compose
docker network create golf-rules-network

# Run with network
docker run --network golf-rules-network --rm -it golf-rules-tools golfqa -i -pg "postgres://golfrag:golfrag@postgres:5432/golfrag?sslmode=disable" -ollama http://ollama:11434
```

## Building from Source

```bash
# Clone the repository
git clone https://github.com/C-Deck/golf-rules-rag.git
cd golf-rules-rag

# Install dependencies
go mod download

# Build the tools
go build -o bin/indexer ./cmd/indexer
go build -o bin/golfqa ./cmd/golfqa
```

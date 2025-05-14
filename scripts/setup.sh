#!/bin/bash

echo "Golf Rules RAG CLI Tool Setup"
echo "----------------------------"

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "Error: Docker is not installed. Please install Docker first."
    exit 1
fi

# Check if Docker Compose is installed
if ! command -v docker-compose &> /dev/null; then
    echo "Error: Docker Compose is not installed. Please install Docker Compose first."
    exit 1
fi

# Check if PDF file is provided
if [ -z "$1" ]; then
    echo "Usage: $0 <path-to-golf-rules-pdf>"
    exit 1
fi

PDF_PATH="$1"
if [ ! -f "$PDF_PATH" ]; then
    echo "Error: PDF file does not exist: $PDF_PATH"
    exit 1
fi

# Start the Docker services
echo "Starting PostgreSQL and Ollama services..."
docker-compose up -d

# Wait for services to be ready
echo "Waiting for services to be ready..."
sleep 10

# Build the Docker image
echo "Building the Golf Rules RAG tools..."
docker build -t golf-rules-tools .

# Run the indexer
echo "Processing the Golf Rules PDF and creating embeddings..."
docker run --network "$(basename "$(pwd)")_default" --rm -v "$(pwd)":/data golf-rules-tools indexer -pdf "/data/$(basename "$PDF_PATH")" -pg "postgres://golfrag:golfrag@postgres:5432/golfrag?sslmode=disable" -ollama "http://ollama:11434"

echo ""
echo "Setup complete! You can now use the Golf Rules RAG CLI tool."
echo ""
echo "To ask a question:"
echo "docker run --network \"$(basename "$(pwd)")_default\" --rm golf-rules-tools golfqa -q \"What is the penalty for a ball hitting another player?\" -pg \"postgres://golfrag:golfrag@postgres:5432/golfrag?sslmode=disable\" -ollama \"http://ollama:11434\""
echo ""
echo "To use interactive mode:"
echo "docker run --network \"$(basename "$(pwd)")_default\" --rm -it golf-rules-tools golfqa -i -pg \"postgres://golfrag:golfrag@postgres:5432/golfrag?sslmode=disable\" -ollama \"http://ollama:11434\""

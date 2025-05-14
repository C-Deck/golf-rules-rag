FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install required packages
RUN apk --no-cache add git gcc musl-dev

# Copy go.mod and go.sum files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the indexer
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/indexer ./cmd/indexer

# Build the CLI tool
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /go/bin/golfqa ./cmd/golfqa

# Create a smaller final image
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /go/bin/indexer /usr/local/bin/indexer
COPY --from=builder /go/bin/golfqa /usr/local/bin/golfqa

# Make binaries executable
RUN chmod +x /usr/local/bin/indexer
RUN chmod +x /usr/local/bin/golfqa

# Use CMD defined when running the container
ENTRYPOINT ["golfqa"]
CMD ["-h"]
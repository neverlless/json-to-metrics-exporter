# Start from golang base image
FROM golang:1.27.3-alpine3.24 AS build

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o json-to-metrics ./cmd/

# Start a new stage from scratch
FROM alpine:3.24.3

WORKDIR /app

# Copy the Pre-built binary file from the previous stage
COPY --from=build /app/json-to-metrics .

# Run as an unprivileged user
USER nobody

EXPOSE 9908

# Command to run the executable
CMD ["./json-to-metrics"]

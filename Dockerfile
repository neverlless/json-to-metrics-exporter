# Start from latest golang base image
FROM golang:1.23.3-alpine AS build

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
RUN go mod download

# Copy the source from the current directory to the Working Directory inside the container
COPY . .

# Build the Go app
RUN go build -o json-to-metrics ./cmd/

# Start a new stage from scratch
FROM alpine:3.12

WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=build /app/json-to-metrics .

# Command to run the executable
CMD ["./json-to-metrics"]

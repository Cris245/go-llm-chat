# syntax=docker/dockerfile:1
# This line specifies the Dockerfile syntax version, enabling BuildKit features.

# Stage 1: Builder
# We use a Go base image with the specified version to compile our application.
FROM golang:1.24.1-alpine AS builder

# Set the working directory inside the container. All subsequent commands will run from here.
WORKDIR /app

# Copy go.mod and go.sum files. These contain our project's dependencies.
# This step is done separately to leverage Docker's build cache. If only source code changes,
# dependencies won't be re-downloaded.[28]
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code into the working directory.
COPY . .

# Build the Go application.
# CGO_ENABLED=0: Disables Cgo, making the binary statically linked and more portable.
# GOOS=linux: Compiles for Linux, as our final image will be Alpine Linux.
# -o /go-llm-chat: Specifies the output binary name and path.
#./cmd/server: Points to our main application entry point.
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/go-llm-chat ./cmd/server

# Stage 2: Final Image
# We use a minimal Alpine Linux image for the final executable.
# This results in a much smaller and more secure final image, as it contains only what's necessary.[28]
FROM alpine:latest

# Set the working directory in the final container.
WORKDIR /root/

# Copy the compiled binary from the 'builder' stage into the final image.
COPY --from=builder /app/go-llm-chat /go-llm-chat

# Expose the port our Go application listens on.
# This informs Docker that the container listens on this port at runtime.
EXPOSE 8080

# Command to run our Go application when the container starts.
CMD ["/go-llm-chat"]
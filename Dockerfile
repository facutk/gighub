# Stage 1: Frontend builder for CSS
# Use a specific node image to build the tailwind css. This separates concerns.
FROM node:20-alpine AS frontend-builder
WORKDIR /app

RUN npm install tailwindcss @tailwindcss/cli

# Copy all source files needed by Tailwind to scan for classes.
# A .dockerignore file is highly recommended to prevent unnecessary files
# (like .git, go.mod, etc.) from being copied, which optimizes caching.
COPY . .

# Generate Tailwind CSS.
# The --minify flag is added to compress the final CSS for production.
RUN npx tailwindcss -i ./assets/css/input.css -o ./assets/css/styles.css --minify

# Stage 2: Backend builder for Go binary
FROM golang:1.23-alpine3.20 AS backend-builder

# Install templ tool
RUN go install github.com/a-h/templ/cmd/templ@latest

WORKDIR /app

# Copy go module files and download dependencies
# This is done first to leverage Docker layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Copy the compiled CSS from the frontend stage
COPY --from=frontend-builder /app/assets/css/styles.css ./assets/css/styles.css

# Hash the CSS file for cache busting and update the Go variable
RUN hash=$(sha256sum assets/css/styles.css | head -c 8) && \
    mv assets/css/styles.css assets/css/styles.$hash.css && \
    echo "package views; var CssPath = \"/assets/css/styles.$hash.css\"" > views/css.go

# Generate templ files
RUN templ generate

# Build the Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -o gighub .

# Stage 3: Create a minimal image to run the application
FROM alpine:3.20

# ARG must be after FROM. This defines a build-time variable that can be passed with --build-arg
ARG GITSHA

# ENV sets the environment variable in the final image. It will be available to the running container.
ENV GITSHA=${GITSHA}

WORKDIR /app

# Add non-root user for security
RUN addgroup -g 1000 appuser && adduser -D -u 1000 -G appuser appuser

# Copy the binary from the builder stage
COPY --from=backend-builder /app/gighub .

# Copy the assets directory (which now contains the compiled CSS)
COPY --from=backend-builder /app/assets ./assets

# Set proper permissions
RUN chown -R appuser:appuser /app

USER appuser

EXPOSE 3000

CMD ["./gighub"]

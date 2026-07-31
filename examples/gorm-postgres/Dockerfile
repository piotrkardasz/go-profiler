FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy root module
COPY go.mod go.sum ./
COPY *.go ./
COPY collector/ collector/
COPY handler/ handler/
COPY storage/ storage/

# Copy gorm collector module
COPY collector/gorm/go.mod collector/gorm/go.sum collector/gorm/
COPY collector/gorm/*.go collector/gorm/

# Copy example
COPY examples/gorm-postgres/ examples/gorm-postgres/

# Download dependencies for the example
WORKDIR /app/examples/gorm-postgres
RUN go mod download
RUN go build -o /app/server .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /server
EXPOSE 8080
CMD ["/server"]

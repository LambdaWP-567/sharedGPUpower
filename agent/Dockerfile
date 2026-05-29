FROM golang:1.24-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /agent ./cmd/agent

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /agent ./agent
COPY configs/agent.yaml.example ./agent.yaml.example

EXPOSE 0
ENTRYPOINT ["./agent", "--config", "/config/agent.yaml"]

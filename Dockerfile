FROM golang:1.26-bullseye AS builder
WORKDIR /workspace
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /workspace/rss-alert ./...

FROM gcr.io/distroless/static
COPY --from=builder /workspace/rss-alert /rss-alert
ENTRYPOINT ["/rss-alert"]

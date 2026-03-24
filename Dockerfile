FROM golang:1.26.1-bookworm AS builder
WORKDIR /workspace
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /workspace/rss-alert ./...

FROM gcr.io/distroless/static
COPY --from=builder /workspace/rss-alert /rss-alert
ENTRYPOINT ["/rss-alert"]

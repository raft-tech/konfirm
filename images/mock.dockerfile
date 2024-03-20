# Build the mock binary
FROM golang:1.21 AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/mock cmd/mock

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o mock ./cmd/mock
FROM gcr.io/distroless/static-debian11:nonroot
COPY --from=builder /workspace/mock /mock
USER 2000:2000
ENTRYPOINT ["/mock"]
CMD ["pass"]
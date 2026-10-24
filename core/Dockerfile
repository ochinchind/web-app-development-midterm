# Build context is the repository root (it needs ./proto and ./core).

# Stage 1: generate Go code from the protos and compile a static binary.
FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler libprotobuf-dev \
    && rm -rf /var/lib/apt/lists/*
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12 \
    && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2

WORKDIR /src/core
COPY core/go.mod core/go.sum ./
RUN go mod download

COPY proto /src/proto
COPY core .
RUN mkdir -p gen && protoc -I ../proto \
        --go_out=gen --go_opt=paths=source_relative \
        --go-grpc_out=gen --go-grpc_opt=paths=source_relative \
        ../proto/cargo/v1/*.proto
RUN CGO_ENABLED=0 go build -o /out/core ./cmd/core

# Stage 2: tiny runtime image.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/core /core
EXPOSE 9090
ENTRYPOINT ["/core"]


FROM debian:bookworm-slim AS descriptor
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler libprotobuf-dev \
    && rm -rf /var/lib/apt/lists/*
COPY proto /proto
RUN protoc -I /proto --include_imports --descriptor_set_out=/proto.pb /proto/cargo/v1/*.proto

FROM envoyproxy/envoy:v1.32.3
COPY envoy/envoy.yaml /etc/envoy/envoy.yaml
COPY --from=descriptor /proto.pb /etc/envoy/proto.pb
CMD ["envoy", "-c", "/etc/envoy/envoy.yaml"]

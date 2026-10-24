
FROM golang:1.26-bookworm AS spec
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler libprotobuf-dev jq \
    && rm -rf /var/lib/apt/lists/*
RUN go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.32.0

COPY proto /proto
COPY swagger/patch.jq /patch.jq
RUN mkdir /out && protoc -I /proto \
        --openapiv2_out=/out \
        --openapiv2_opt=allow_merge=true,merge_file_name=cargo,json_names_for_fields=false,disable_default_errors=true \
        /proto/cargo/v1/*.proto

RUN jq -f /patch.jq /out/cargo.swagger.json > /out/openapi.json

FROM swaggerapi/swagger-ui:v5.17.14
COPY --from=spec /out/openapi.json /spec/openapi.json
ENV SWAGGER_JSON=/spec/openapi.json

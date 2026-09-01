# syntax = docker/dockerfile:1.22
########################################

FROM --platform=${BUILDPLATFORM} golang:1.27.0-alpine AS builder
RUN apk update && apk add --no-cache make
ENV GO111MODULE=on
WORKDIR /src

COPY ["go.mod", "go.sum", "/src/"]
RUN go mod download && go mod verify

COPY . .
ARG VERSION
ARG TAG
ARG SHA
RUN make build-all-archs

########################################

FROM --platform=${TARGETARCH} scratch AS mimiops-mcp
LABEL org.opencontainers.image.source="https://github.com/sergelogvinov/mimiops-mcp" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.description="Mimi Ops is an opinionated MCP server for Kubernetes"

COPY --from=gcr.io/distroless/static-debian13:nonroot . .
ARG TARGETARCH
COPY --from=builder /src/bin/mimiops-mcp-${TARGETARCH} /bin/mimiops-mcp

ENTRYPOINT ["/bin/mimiops-mcp"]

########################################

FROM --platform=${TARGETARCH} scratch AS release

COPY --from=gcr.io/distroless/static-debian13:nonroot . .
ARG TARGETPLATFORM
COPY ${TARGETPLATFORM}/bin/mimiops-mcp /bin/mimiops-mcp

ENTRYPOINT ["/bin/mimiops-mcp"]

# Build stage - use native platform for builder to avoid emulation
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi9/go-toolset:1.25@sha256:ca9697879a642fa691f17daf329fc24d239f5b385ecda06070ebddd5fdab287d AS builder

# Build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH

# Switch to root for installation
USER root

# Install make (using yum for go-toolset image)
RUN yum install -y make && yum clean all

WORKDIR /workspace

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./

# Enable Go toolchain auto-download to match go.mod version requirement
ENV GOTOOLCHAIN=auto
RUN go mod download

# Copy source code and Makefile
COPY . .

# Build arguments for version information
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

# Build using Makefile with cross-compilation
RUN make build \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    VERSION=${VERSION} \
    COMMIT=${COMMIT} \
    DATE=${DATE}



# Runtime stage
FROM registry.redhat.io/openshift4/ose-cli-rhel9:v4.21.0@sha256:463eb49fab8d00b81352f9fce7bc9ccb64898ba2a044408b6f4b7bf56d1b5c8c

# Build arguments for downloading architecture-specific binaries
ARG TARGETARCH

# Set default KUBECONFIG path for container usage
# Users can override this with -e KUBECONFIG=<path> when running the container
ENV KUBECONFIG=/kubeconfig

# Copy upgrade helpers from submodule folder
COPY ./rhoai-upgrade-helpers /opt/rhai-upgrade-helpers

# Copy requirements.txt
COPY ./requirements.txt ./requirements.txt

# Install base utilities (jq, wget, python3, python3-pip)
RUN yum install -y \
    gcc \
    jq \
    wget \
    python3 \
    python3-pip \
    && yum clean all

# Python deps for ray_cluster_migration.py (kubernetes, PyYAML)
RUN python3 -m pip install --use-pep517 -r requirements.txt

# Copy binary from builder (cross-compiled for target platform)
COPY --from=builder /workspace/bin/kubectl-odh /opt/rhai-cli/bin/rhai-cli

# Add rhai-cli to PATH
ENV PATH="/opt/rhai-cli/bin:${PATH}"

# Set entrypoint to rhai-cli binary
# Users can override with --entrypoint /bin/bash for interactive debugging
ENTRYPOINT ["/opt/rhai-cli/bin/rhai-cli"]

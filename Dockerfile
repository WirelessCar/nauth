FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder
ARG TARGETOS
ARG TARGETARCH
# Go Cryptographic Module used for the build.
# - off (default): standard image.
# - a frozen, CMVP-validated snapshot such as v1.0.0: FIPS image. The binary then defaults to
#   GODEBUG=fips140=on.
ARG GOFIPS140=off

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN case "${GOFIPS140}" in \
      off|v[0-9]*) ;; \
      *) echo "GOFIPS140 must be 'off' or a frozen module version (e.g. v1.0.0), got '${GOFIPS140}'"; exit 1 ;; \
    esac && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} GOFIPS140=${GOFIPS140} \
    go build -trimpath -a -o manager ./cmd

# Fail the build if the FIPS settings did not land in the binary.
RUN if [ "${GOFIPS140}" != "off" ]; then \
      go version -m manager | grep -q "GOFIPS140=${GOFIPS140}" && \
      go version -m manager | grep -q "DefaultGODEBUG=.*fips140=on" \
      || { echo "FIPS build verification failed:"; go version -m manager; exit 1; }; \
    fi

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG GOFIPS140=off
LABEL io.nauth.fips140="${GOFIPS140}"
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]

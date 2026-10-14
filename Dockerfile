# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build
# Import BuildKit's automatic target values without overriding them with defaults.
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o /out/thothmemory ./cmd/thothmemory
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o /out/thothmemory.exe ./cmd/thothmemory
FROM scratch
LABEL org.opencontainers.image.title="ThothMemory" org.opencontainers.image.description="Local-first shared engineering memory for AI agents" org.opencontainers.image.source="https://github.com/Lord1Egypt/ThothMemory" org.opencontainers.image.licenses="MIT" org.opencontainers.image.version="0.2.3"
COPY --from=build /out/thothmemory /usr/local/bin/thothmemory
COPY --from=build /out/thothmemory.exe /clients/thothmemory.exe
COPY LICENSE /licenses/ThothMemory.txt
COPY third_party/ /licenses/dependencies/
USER 65532:65532
EXPOSE 7319
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s CMD ["/usr/local/bin/thothmemory","healthcheck"]
ENTRYPOINT ["/usr/local/bin/thothmemory"]
CMD ["serve"]

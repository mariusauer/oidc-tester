# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.26.1-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY templates ./templates

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/oidc-tester .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/oidc-tester /oidc-tester

USER 65532:65532
EXPOSE 3000
ENTRYPOINT ["/oidc-tester"]

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/go-liga ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/go-liga /go-liga
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/go-liga"]

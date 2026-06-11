# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src
# Cache modules first.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
# Pure-Go build (modernc sqlite not used; pgx is pure Go) → static binary.
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/ImErdis/xray-api/internal/version.Version=${VERSION} -X github.com/ImErdis/xray-api/internal/version.Commit=${COMMIT}" \
    -o /out/xray-api ./cmd/xray-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/xray-api /usr/local/bin/xray-api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/xray-api"]
CMD ["serve"]

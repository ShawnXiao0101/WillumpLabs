FROM node:22-alpine AS web-builder
WORKDIR /src/web

COPY web/package*.json ./
RUN npm ci

COPY web ./
RUN npm run build

FROM golang:1.26-alpine AS go-builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY --from=web-builder /src/web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/willumplabs ./cmd/server

FROM alpine:3.22
WORKDIR /app

RUN apk add --no-cache ca-certificates wget

COPY --from=go-builder /out/willumplabs ./willumplabs
COPY --from=go-builder /src/web/dist ./web/dist

ENV PORT=8080
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1

CMD ["./willumplabs"]

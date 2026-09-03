# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@10 --activate
COPY web/frontend/package.json web/frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/frontend/ ./
RUN printf 'VITE_API_BASE=\nVITE_CF_WEB_ANALY_TOKEN=\nVITE_IS_TELEGRAM=false\n' > .env.prod \
    && npx vite build -m prod --outDir /out --emptyOutDir

FROM golang:1.25-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN rm -rf web/dist && mkdir -p web/dist
COPY --from=frontend /out/ web/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /tempmail ./cmd/tempmail

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && mkdir -p /data
COPY --from=backend /tempmail /usr/local/bin/tempmail
ENV DB_PATH=/data/tempmail.db HTTP_ADDR=:8080 SMTP_ADDR=:25
EXPOSE 8080 25
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health_check || exit 1
ENTRYPOINT ["tempmail"]

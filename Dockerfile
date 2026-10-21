# Builds the Mini App and the API into one image that serves both from a
# single origin. The SQLite file lives on a volume at /data.

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json web/.npmrc ./
RUN npm ci
COPY web ./
RUN npm run build

FROM golang:1.27-alpine AS api
WORKDIR /src/api
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /kcal-bot .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 kcal \
 && mkdir -p /data && chown kcal:kcal /data
WORKDIR /app
COPY --from=api /kcal-bot /app/kcal-bot
COPY --from=web /src/web/dist /app/web
USER kcal
VOLUME ["/data"]
EXPOSE 8080
ENV STATIC_DIR=/app/web
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1
ENTRYPOINT ["/app/kcal-bot"]
CMD ["-db", "/data/kcal.db", "-addr", ":8080"]

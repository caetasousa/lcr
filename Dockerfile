# syntax=docker/dockerfile:1

# ---------- 1) Frontend React (Vite) ----------
FROM node:26-alpine AS frontend
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---------- 2) API Go ----------
FROM golang:1.27-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lcr ./cmd/server

# ---------- 3) Imagem final ----------
FROM alpine:3.24
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=backend /out/lcr /app/lcr
COPY --from=frontend /web/dist /app/web
ENV STATIC_DIR=/app/web PORT=8080
USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s CMD wget -qO- http://127.0.0.1:8080/api/ac >/dev/null || exit 1
ENTRYPOINT ["/app/lcr"]

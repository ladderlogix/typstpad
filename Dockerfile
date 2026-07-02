# ---- frontend build ----
FROM node:22-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
RUN npm run build

# ---- go build ----
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/embed.go web/
COPY --from=web /src/web/dist web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /typstpad ./cmd/typstpad

# ---- runtime ----
FROM debian:bookworm-slim
ARG TYPST_VERSION=0.15.0
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl xz-utils \
      fonts-liberation fonts-noto-core fonts-dejavu-core \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/typst-x86_64-unknown-linux-musl.tar.xz" \
      | tar -xJ -C /tmp \
    && mv /tmp/typst-x86_64-unknown-linux-musl/typst /usr/local/bin/typst \
    && rm -rf /tmp/typst-x86_64-unknown-linux-musl \
    && typst --version
RUN useradd -m -u 10001 typstpad
COPY --from=build /typstpad /usr/local/bin/typstpad
USER typstpad
ENV DATA_DIR=/data
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["typstpad"]
CMD ["serve"]

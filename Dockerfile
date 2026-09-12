# Stage 1: Build frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --ignore-scripts --registry=https://registry.npmmirror.com
COPY frontend/ ./
RUN npm run build

# Stage 2: Build backend
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS backend-builder
ARG TARGETARCH
WORKDIR /app
COPY go.mod go.sum ./
ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=off
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -ldflags "-s -w" -o oci-panel main.go

# Stage 3: Final minimal image
FROM alpine:3.21
WORKDIR /app
RUN sed -i 's#dl-cdn.alpinelinux.org#mirrors.aliyun.com#g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata
COPY --from=backend-builder /app/oci-panel .
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
COPY config.toml.example ./config.toml
EXPOSE 8999
CMD ["./oci-panel"]

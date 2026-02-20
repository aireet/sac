# Build stage
FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/library/golang:1.25.7-alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /build

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o migrate ./cmd/migrate

# Runtime stage
FROM swr.cn-north-4.myhuaweicloud.com/ddn-k8s/docker.io/library/alpine:3.19

RUN sed -i 's#https://dl-cdn.alpinelinux.org#https://mirrors.aliyun.com#g' /etc/apk/repositories && \
    apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/migrate .

ENTRYPOINT ["./migrate"]

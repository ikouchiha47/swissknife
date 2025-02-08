FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /tmp/swissknife ./cmd/paged/main.go

FROM gruebel/upx:latest AS compressor

COPY --from=builder /tmp/swissknife /tmp/swissknife

RUN upx --best --lzma /tmp/swissknife

FROM alpine:latest

COPY --from=compressor /tmp/swissknife /output/swissknife

# CMD ["tail", "-f", "/dev/null"]

FROM golang:1.26-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=${VERSION} -s -w" -o /out/excelsior ./cmd/excelsior

FROM alpine:3.20
RUN apk add --no-cache ca-certificates git bash
RUN adduser -D -s /bin/sh app
USER app
WORKDIR /workspace
COPY --from=builder /out/excelsior /usr/local/bin/excelsior
ENTRYPOINT ["excelsior"]
CMD ["--help"]

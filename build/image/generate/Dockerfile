FROM golang:1.20

RUN go install github.com/daixiang0/gci@v0.3.3 && \
    go clean -cache

ENV GOROOT=/usr/local/go

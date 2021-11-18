FROM golang:1.16 as builder

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o mailroom ./cmd/mailroom/main.go

RUN apt update && apt install -y curl

RUN export GOFLOW_VERSION=$(grep goflow go.mod | cut -d" " -f2 | head -n 1) && \
                   curl -L https://github.com/nyaruka/goflow/releases/download/${GOFLOW_VERSION}/docs.tar.gz | tar zxv && \
                   cp ./docs/en-us/*.* docs/

FROM alpine:3.7

ENV USER=mailroom
ENV UID=13337
ENV GID=13337

RUN addgroup -g "$GID" "$USER" \
    && adduser \
    -D \
    -g "" \
    -h "$(pwd)" \
    -G "$USER" \
    -H \
    -u "$UID" \
    "$USER"

RUN apk update && apk add ca-certificates tzdata && rm -rf /var/cache/apk/*

WORKDIR /app

COPY --from=builder /app/mailroom .
COPY --from=builder /app/docs ./docs

EXPOSE 8080

USER mailroom

ENTRYPOINT []
CMD ["/app/mailroom"]
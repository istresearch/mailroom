FROM golang:1.14 as builder

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o mailroom ./cmd/mailroom/main.go

RUN apt update && apt install -y curl

RUN export GOFLOW_VERSION=$(grep goflow go.mod | cut -d" " -f2 | head -n 1) && \
 curl https://codeload.github.com/nyaruka/goflow/tar.gz/$GOFLOW_VERSION | tar --wildcards --strip=2 -zx "*/docs/en_US/*" \
 && mv en_US docs

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
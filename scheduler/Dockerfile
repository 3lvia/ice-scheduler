FROM golang:1.24-alpine AS builder
LABEL maintainer="elvia@elvia.no"

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

WORKDIR /app
COPY . .

RUN go mod download
RUN go build -o ./out/scheduler ./cmd/scheduler/

FROM alpine:3.21 AS runtime
LABEL maintainer="elvia@elvia.no"

RUN addgroup application-group --gid 1001 \
    && adduser application-user --uid 1001 \
    --ingroup application-group \
    --disabled-password

RUN apk add --no-cache ca-certificates apache2-utils

WORKDIR /app
COPY --from=builder /app/out .
RUN chown --recursive application-user .
USER application-user

EXPOSE 8080

CMD ["/app/scheduler"]
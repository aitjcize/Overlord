FROM golang:alpine AS builder

RUN mkdir -p /src
WORKDIR /src

COPY . .

RUN apk update && apk add make gcc linux-headers libc-dev

RUN make

FROM alpine:latest

RUN mkdir -p /overlord/config

COPY --from=builder /src/bin/overlordd /overlord
COPY --from=builder /src/overlord/app /overlord/app

EXPOSE 4455 4456 9000

CMD ["/overlord/overlordd", "-tls=/config/cert.pem,/config/key.pem", \
     "-htpasswd-path=/config/overlord.htpasswd"]

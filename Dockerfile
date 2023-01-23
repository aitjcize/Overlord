FROM golang:alpine AS builder

RUN mkdir -p /src
WORKDIR /src

COPY . .

RUN apk update && apk add make gcc linux-headers libc-dev

RUN make

FROM alpine:latest

RUN mkdir /config /overlord

COPY --from=builder /src/bin/overlordd /overlord
COPY --from=builder /src/bin/ghost /overlord
COPY --from=builder /src/overlord/app /overlord/app
COPY --from=builder /src/scripts/start_overlordd.sh /overlord

ENV SHELL=/bin/sh

EXPOSE 4456 80

CMD ["/overlord/start_overlordd.sh"]

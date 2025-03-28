# Build go app.

FROM crazymax/osxcross:latest-alpine AS osxcross
FROM golang:alpine AS gobuilder

RUN mkdir -p /src
WORKDIR /src
COPY . .

RUN apk update && apk add make gcc linux-headers libc-dev clang lld musl-dev

RUN make STATIC=true build-go
RUN --mount=type=bind,from=osxcross,source=/osxcross,target=/osxcross \
    make ghost-darwin

FROM python:alpine AS pybuilder

RUN mkdir -p /src
WORKDIR /src
COPY . .

RUN apk update && apk add make git binutils
RUN make build-py


# Build node app.
FROM node:23-alpine AS nodebuilder

RUN mkdir -p /src
WORKDIR /src
COPY . .

RUN apk update && apk add make
RUN make build-apps


# Build final image.
FROM alpine:latest

RUN mkdir -p /config /app /app/webroot/upgrade
WORKDIR /app

COPY --from=gobuilder /src/bin/overlordd /app
COPY --from=gobuilder /src/sh/start_overlordd.sh /app
COPY --from=gobuilder /src/bin/ghost.* /app/webroot/upgrade/
COPY --from=pybuilder /src/bin/ghost.* /app/webroot/upgrade/
COPY --from=pybuilder /src/bin/ovl.* /app/webroot/upgrade/

COPY --from=nodebuilder /src/webroot /app/webroot

ENV SHELL=/bin/sh

EXPOSE 4456 80

CMD ["/app/start_overlordd.sh"]

# Build go app.
FROM golang:alpine AS gobuilder

RUN mkdir -p /src
WORKDIR /src
COPY . .

RUN apk update && apk add make gcc linux-headers libc-dev
RUN make STATIC=true build-go


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
COPY --from=gobuilder /src/bin/ghost /app
COPY --from=gobuilder /src/scripts/start_overlordd.sh /app
COPY --from=gobuilder /src/bin/ghost /app/webroot/upgrade/ghost.linux.amd64

RUN sha1sum /app/webroot/upgrade/ghost.linux.amd64 | \
	awk '{ print $1 }' > /app/webroot/upgrade/ghost.linux.amd64.sha1

COPY --from=nodebuilder /src/webroot /app/webroot

ENV SHELL=/bin/sh

EXPOSE 4456 80

CMD ["/app/start_overlordd.sh"]

#!/bin/sh

APP_DIR="/app"
CONFIG_DIR="/config"
OPTS=""

if [ -e "${CONFIG_DIR}/cert.pem" ] && [ -e "${CONFIG_DIR}/key.pem" ]; then
  OPTS="$OPTS -tls=${CONFIG_DIR}/cert.pem,${CONFIG_DIR}/key.pem"
fi

if [ -e "${CONFIG_DIR}/overlord.db" ]; then
  OPTS="$OPTS -db-path=${CONFIG_DIR}/overlord.db"
fi

echo "Starting overlrodd with args: $OPTS ..."
exec ${APP_DIR}/overlordd $OPTS

#!/bin/sh

CONFIG_DIR="/config"
OPTS=""

if [ -e "${CONFIG_DIR}/cert.pem" ] && [ -e "${CONFIG_DIR}/key.pem" ]; then
  OPTS="$OPTS -tls=${CONFIG_DIR}/cert.pem,${CONFIG_DIR}/key.pem"
fi

if [ -e "${CONFIG_DIR}/overlord.htpasswd" ]; then
  OPTS="$OPTS -htpasswd-path=${CONFIG_DIR}/overlord.htpasswd"
fi

echo "Starting overlrodd with args: $OPTS ..."
exec /overlord/overlordd $OPTS

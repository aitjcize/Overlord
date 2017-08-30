#!/bin/sh
# Copyright 2016 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

set -e

# This is a simple setup script that would interactively setup login
# credential and SSL certificate for Overlord.

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
CONFIG_DIR="${SCRIPT_DIR}/config"

setup_login() {
  htpasswd_path="${CONFIG_DIR}/overlord.htpasswd"

  echo "Setting up Overlord login credentials."
  echo "This username / password would be used to login to overlord" \
    "web interface."
  echo

  printf "Enter username: "
  read -r username

  htpasswd -B -c "${htpasswd_path}" "${username}"

  echo "Login credentials for user ${username} is added."
}

setup_ssl() {
  key_path="${CONFIG_DIR}/key.pem"
  cert_path="${CONFIG_DIR}/cert.pem"

  echo "Setting up Overlord SSL certificates."
  echo

  printf "Enter the FQDN / IP for the server running Overlord: "
  read -r common_name

  openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "${key_path}" -out "${cert_path}" -days 365 \
    -subj "/CN=${common_name}"

  echo "SSL certificates generated."
}

main() {
  setup_login
  echo
  setup_ssl
}

main "$@"

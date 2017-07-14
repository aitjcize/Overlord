# Copyright 2016 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

# This is the image for building Overlord.
# The reason to separate them is because building requires golang and gcc,
# which is useless after the build.

FROM golang:alpine
MAINTAINER Pi-Hsun Shih <pihsun@google.com>

ARG workdir="/usr/src/app"
WORKDIR "${workdir}"

RUN apk add --no-cache \
  build-base \
  git

# Use same structure as in the original tree, so we can use Makefile of
# overlord easily.
COPY go "${workdir}/go/"
RUN mkdir -p "${workdir}/go/bin/"

# build
ARG output_file="overlord.tar.gz"
RUN make -C "${workdir}/go/src/overlord" package PACKAGE_OUTPUT_PATH="${workdir}/${output_file}"

# nothing to do here
CMD ["echo"]

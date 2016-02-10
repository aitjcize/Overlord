#!/usr/bin/python -u
# -*- coding: utf-8 -*-
#
# Copyright 2016 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# Stream Camera feed with ffmpeg and forward the data to standard output

import argparse
import atexit
import BaseHTTPServer
import platform
import struct
import subprocess
import sys


_SERVER_PORT = 8080
_BUFSIZ = 8192
_DEFAULT_DEVICE = '/dev/video0'
_DEFAULT_SIZE = '640x480'
_DEFAULT_BITRATE = '800k'
_DEFAULT_FRAMERATE = 30


class ForwardToStdoutRequestHandler(BaseHTTPServer.BaseHTTPRequestHandler):
  def do_POST(self):
    size = self.server.size.split('x')
    width = int(size[0])
    height = int(size[1])

    # Write jsmpeg header
    sys.stdout.write('jsmp' + struct.pack('>2H', width, height))
    sys.stdout.flush()

    # Forward video stream to stdout
    while True:
      data = self.rfile.read(_BUFSIZ)
      if len(data) == 0:
        break
      sys.stdout.write(data)
      sys.stdout.flush()


def StartCaptureProcess(args):
  """Start the video capture process.

  Start ffmpeg for encoding video with v3l2 than output to the web server
  located at localhost:_SERVER_PORT

  Returns:
    handler for the subprocess
  """
  system = platform.system()

  if system == "Linux":
    return subprocess.Popen(
        'sleep 1; '
        'ffmpeg -an -s %s -f video4linux2 -i %s -f mpeg1video -b:v %s -r %d '
        'http://localhost:%s/' % (args.size, args.device, args.bitrate,
                                  args.framerate, _SERVER_PORT),
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, shell=True)
  elif system == "Darwin":
    return subprocess.Popen(
        'sleep 1; '
        'ffmpeg -an -f avfoundation -video_size %s -framerate %d -i %s -b:v %s '
        '-f mpeg1video -r %d http://localhost:%s/' %
        (args.size, args.framerate, args.device, args.bitrate, args.framerate,
         _SERVER_PORT),
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, shell=True)


def StopCaptureProcess(handler):
  handler.kill()


def main():
  parser = argparse.ArgumentParser()
  parser.add_argument('--device', dest='device', default=_DEFAULT_DEVICE,
                      help='Video device to capture video from')
  parser.add_argument('--size', dest='size', default=_DEFAULT_SIZE,
                      help='Resolution of the video stream')
  parser.add_argument('--bitrate', dest='bitrate', default=_DEFAULT_BITRATE,
                      help='Bitrate of the video stream')
  parser.add_argument('--framerate', type=int, dest='framerate',
                      default=_DEFAULT_FRAMERATE,
                      help='Framerate of the video stream')
  args = parser.parse_args()

  handler = StartCaptureProcess(args)
  atexit.register(StopCaptureProcess, handler)

  server = BaseHTTPServer.HTTPServer(
      ('localhost', _SERVER_PORT), ForwardToStdoutRequestHandler)
  server.size = args.size
  server.serve_forever()


if __name__ == '__main__':
  main()

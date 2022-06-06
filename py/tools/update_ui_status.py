#!/usr/bin/python
# Copyright 2015 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""This is a tool for Overlord fixtures, the script will keep updating light
status by sending corresponding commands. The commands are read from a
configuration file, default is "/run/factory/goofy_ghost.json". The
configuration file should specify a list of commands to execute and the polling
interval for each command.
"""

import argparse
import json
import logging
from queue import PriorityQueue
import subprocess
import sys
import time


def main():
  parser = argparse.ArgumentParser(description=__doc__)
  parser.add_argument(
      '-c',
      '--config',
      default='/run/factory/goofy_ghost.json',
      help=('Specify path to the config file, '
            'default file: /run/factory/goofy_ghost.json'))

  args = parser.parse_args()

  with open(args.config, encoding='utf8') as f:
    properties = json.load(f)
    try:
      lights = properties['ui']['lights']
    except Exception:
      lights = []
      logging.warning("Can't find ui > lights entry in `%s'", args.config)

    try:
      data = properties['ui']['display']['data']
    except Exception:
      data = []
      logging.warning("Can't find ui > display > data entry in `%s'",
                      args.config)

    items = lights + data
    queue = PriorityQueue(len(items))

    for item in items:
      if 'poll' in item:
        poll = item['poll']
        poll['interval'] = min(poll.get('interval', 0), 10000)
        queue.put((time.time(), poll))
      if 'init_cmd' in item:
        subprocess.call(item['init_cmd'], shell=True)

    if queue.empty():
      sys.exit(0)

    try:
      while True:
        (when, poll) = queue.get_nowait()
        if time.time() < when:  # not now
          queue.put((when, poll))
          sleep_time = when - time.time()
          if sleep_time > 0:
            time.sleep(sleep_time)
        else:
          subprocess.call(poll['cmd'], shell=True)
          queue.put((time.time() + (poll['interval'] / 1000.0), poll))
    except (KeyboardInterrupt, SystemExit):
      pass


if __name__ == '__main__':
  main()

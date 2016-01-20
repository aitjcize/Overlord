#!/usr/bin/python -u
# -*- coding: utf-8 -*-
#
# Copyright 2016 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# relay_overlord_discovery_packet is a tool for relaying Overlord LAN discovery
# message from one interfaces(subnets) to other interfaces(subnets).

from __future__ import print_function

import fcntl
import logging
import select
import socket
import struct
import sys
import SocketServer


IFNAMSIZ = 16
SIOCGIFADDR = 0x8915
SIOCSIFNETMASK = 0x891b

_OVERLORD_LAN_DISCOVERY_PORT = 4456
_BUFSIZE = 8192


def GetIPs(interfaces):
  """Prepare interface IPs required for the RelayOverlordDiscoveryPacket
  function.

  Returns:
    A tuple where the first element is a list of IPs for *interfaces*, and the
    second element is a list of broadcast IPs for *interfaces*.
  """
  s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)

  broadcast_ips = []
  interface_ips = []

  for interface in interfaces:
    try:
      ip = fcntl.ioctl(s.fileno(), SIOCGIFADDR,
                       struct.pack('256s', interface[:IFNAMSIZ]))[20:24]
      netmask = fcntl.ioctl(s.fileno(), SIOCSIFNETMASK,
                            struct.pack('256s', interface[:IFNAMSIZ]))[20:24]
      interface_ips.append(socket.inet_ntoa(ip))

      # Convert to int from network byte order (big-endian)
      ip_int = struct.unpack('>i', ip)[0]
      netmask_int = struct.unpack('>i', netmask)[0]

      broadcast_ip_int = ip_int | ~netmask_int
      broadcast_ip = socket.inet_ntoa(struct.pack('>i', broadcast_ip_int))
      broadcast_ips.append(broadcast_ip)
    except IOError as e:
      logging.error('error: %s, %s', interface, e)

  return interface_ips, broadcast_ips


class OverlordLANDiscoveryRelayServer(SocketServer.UDPServer):
  def __init__(self, interfaces, *args, **kwargs):
    SocketServer.UDPServer.__init__(self, *args, **kwargs)

    self.send_sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    self.send_sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)

    interface_ips, broadcast_ips = GetIPs(interfaces)
    self.interface_ips = interface_ips
    self.broadcast_ips = broadcast_ips

  def server_bind(self):
    SocketServer.UDPServer.server_bind(self)
    self.socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)


class OverlordLANDiscoveryRelayHandler(SocketServer.BaseRequestHandler):
  def handle(self):
    data = self.request[0]

    if self.client_address[0] in self.server.interface_ips:
      return

    parts = data.split()
    if parts[0] == 'OVERLORD':
      ip, port = parts[1].split(':')
      if not ip:
        ip = self.client_address[0]

      msg = 'OVERLORD %s:%s' % (ip, port)
      for ip in self.server.broadcast_ips:
        self.server.send_sock.sendto(msg, (ip, _OVERLORD_LAN_DISCOVERY_PORT))


def main():
  logging.basicConfig(level=logging.INFO)

  interfaces = sys.argv[1:]
  if not interfaces:
    print('Usage: %s [interface] ...' % sys.argv[0])
    sys.exit(1)

  server = OverlordLANDiscoveryRelayServer(
      interfaces,
      ('0.0.0.0', _OVERLORD_LAN_DISCOVERY_PORT),
      OverlordLANDiscoveryRelayHandler)
  server.serve_forever()


if __name__ == '__main__':
  main()

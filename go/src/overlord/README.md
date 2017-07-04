# Overlord Deployment Guide

# Objective

The goal of this document is to provide deployment guidance to the
[Overlord](http://go/overlord-doc) factory monitor system.

# Basic Deployment

## Get the source

The Overlord source code resides in the chromiumos factory repository. You can
clone the factory git repo without the entire chromiumos source tree:

```bash
$ git clone https://chromium.googlesource.com/chromiumos/platform/factory
```

## Build

Make sure you have the Go lang toolchain installed (`apt-get install
google-golang`). Building Overlord is as easy as typing `make` under the
`go/src/overlord`

```bash
$ cd factory/go/src/overlord

$ make
```

## Deploy

Assuming your server is located at `SHOPFLOOR_IP`, just scp the `go/bin` dir
onto the server

```bash
$ scp -r factory/go/bin SHOPFLOOR_IP:~/overlord
```

In this particular case, we copy the bin folder onto the HOME/overlord on the
shopfloor server.

## Start the Server

To start the server, simply run the overlordd binary

```bash
shopfloor:~ $ cd overlord

shopfloor:~/overlord $ nohup ./overlordd &
```

One the server started, browse `http://SHOPFLOOR_IP:9000` to access the
overlord web frontend.

There are some options available for the overlordd command, use `overlord
-help` to see them.

By default, overlordd is started with HTTP basic auth enabled. The default
account / password is `overlord/cros`, **please follow the later section in
this document to change the password for production environment**. To disable
HTTP basic auth, simply add the `-no-auth` option when launching overlordd.
**This is strongly discouraged in production environment**, as it expose your
server for anyone to access it.

## Ghost Clients

The clients are called `ghost` in the Overlord framework. There are currently
two implementations, one implemented in python and the other implemented in go.
The python version can be found under the factory source repository:
`py/tools/ghost.py`; while the go version is under `go/src/overlord/ghost.go`
which is built along side with the `overlordd` binary under `go/bin`.

# Server Configuration

## Changing LAN Discovery Broadcast Interface

Overlord server broadcasts LAN discovery messages into the subnet so clients
can identify the server IP. In a typical factory network, a server might have
at least two interface, one for LAN and another for external network. By
default, overlord broadcast the LAN discovery message to the default gateway's
subnet. To specify a different one, use the `-lan-disc-iface` option:

```bash
shopfloor:~/overlord $ nohup ./overlordd -lan-disc-iface=eth1 &
```

## Changing Default Password

The password to the Overlord server is stored in a apache style htpasswd file.
This means it supports multiple login credentials. To change it, first remove
the default password:

```bash
shopfloor:~/overlord $ rm app/overlord.htpasswd
```

The use the htpasswd utility to create a new htpasswd file. The htpasswd
utility can be installed by `apt-get install apache2-utils`:

```bash
shopfloor:~/overlord $ htpasswd -B -c app/overlord.htpasswd username11

New password:
Re-type new password:
Updating password for user username1
```

The `-c` option create the new file. To add more credentials to the file simply
remove the `-c` option:

```bash
shopfloor:~/overlord $ htpasswd -B app/overlord.htpasswd username2
```

# Enable SSL Support

To ensure the privacy of the communication with the overlord server via the web
frontend. It's encouraged to enable SSL on a production environment.

You can either use a CA-signed SSL certificate or a self-signed SSL
certificate. We usually use a self-signed certificate in the factory since we
can't be 100% sure we are the only one that have access to the server. If the
partner have access to the server, they can steal your SSL certificate file!

To generate a self-signed SSL certificate, you need the openssl software suite
from you distribution:

```bash
shopfloor:~/overlord $ openssl req -x509 -nodes -newkey rsa:2048 -keyout
key.pem -out cert.pem -days 365
```

**Note that you need to input the correct `common name` when generating the
certificate, `common name` is typically the  FQDN or IP of the server.**

This will generate two files: `cert.pem` and `key.pem`. Assign it to the `-tls`
option when starting overlordd, and you are all set.

```bash
shopfloor:~/overlord $ nohup ./overlordd -tls=cert.pem,key.pem &
```

Now you can browse `https://shopfloor_ip:9000` to access the web frontend.
Note that you need to use `https` instead of `http`.

**Connecting to a TLS enabled Server**

ghost automatically detects if Overlord server has TLS enabled and verify it
using system installed ca-certificates bundle.

To connect to a TLS enabled Overlord server, a ghost client must specify the
TLS certificate to be used for verification:

```bash
$ ghost --tls-cert-file cert.pem SERVER_IP
```

Optionally, one could enable TLS but skip certificate verification:

```bash
$ ghost --tls-no-verify SERVER_IP
```

## Caveats

The SSL certification generated above is a self-signed SSL certificate. The
first time you visit the web frontend, you will see a warning.

This is the result of self-signed SSL certificate, no need to panic. Click on
the left top corner of the browser to see the certificate information.

**Make sure the fingerprint is correct** then hit the `Advanced` button then
`Proceed`.

# Auto Upgrade Setup

Overlord supports an AU(Auto Upgrade) protocol for updating ghost clients.
Ghost clients automatically check for update on registration. Admins can also
force an upgrade if there are updates available.

## Prepare Upgrade Files

Fetch the latest ghost.py or ghost binary from factory repo. For the ghost
binary, rename it into ghost.ARCH, where `ARCH` is go runtime.GOARCH variable
on that platform. For a x86\_64 platform, the runtime.GOARCH equals `amd64`. In
such case, rename the binary to `ghost.amd64`.

## Copy the Upgrade File Onto the Server

### Create the required directory structure

On the Shopfloor server:

```bash
shopfloor:~/overlord $ mkdir app/upgrade
```

### Copy the Upgrade file

```bash
$ scp ghost.py SHOPFLOOR_IP:~/overlord/app/upgrade

$ scp ghost.amd64 SHOPFLOOR_IP:~/overlord/app/upgrade
```

### Generate Checksum

```bash
shopfloor:~/overlord $ cd app/upgrade

shopfloor:~/overlord/app/upgrade $ for i in `ls ghost.* | grep -v sha1`; do \
        sha1sum $i | awk '{ print $1 }' > $i.sha1 done
```

### Force Upgrade

After the above step, the upgrade files are ready. Now if a new client connects
or client reconnects to the overlord server, it automatically checks for
upgrade and apply it. To force an upgrade for already connected clients, simply
send a GET request to the server:

```bash
$ curl -k -u username1:password1 'https://localhost:9000/api/agents/upgrade'
```

(Note: use `http` if you don't have SSL enabled)

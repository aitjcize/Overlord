Overlord Advanced Deployment
============================

Changing LAN Discovery Broadcast Interface
------------------------------------------

Overlord server broadcasts LAN discovery messages into the subnet so clients can identify the server IP.  By default, overlord broadcast the LAN discovery message to the default gateway`s subnet.  To specify a different one, use the ``-lan-disc-iface`` option:

.. code-block:: bash

   % nohup ./overlordd -lan-disc-iface=eth1 &

Changing Default Password
-------------------------
The password to the Overlord server is stored in an apache style htpasswd file.  This means it supports multiple login credentials.  To change it, first remove the default password:

.. code-block:: bash

  % rm app/overlord.htpasswd

The use the htpasswd utility to create a new htpasswd file.  The htpasswd utility can be installed by ``apt-get install apache2-utils``:

.. code-block:: bash

   % htpasswd -B -c app/overlord.htpasswd username1
   New password: 
   Re-type new password: 
   Updating password for user username1

The ``-c`` option create the new file.  To add more credentials to the file simply remove the ``-c`` option:

.. code-block:: bash

   % htpasswd -B app/overlord.htpasswd username2

Enable SSL Support
------------------
To ensure the privacy of the communication with the overlord server via the web frontend.  It`s encouraged to enable SSL in a production environment.

You can either use a CA-signed SSL certificate or a self-signed SSL certificate.

To generate a self-signed SSL certificate, you need the OpenSSL software suite from your distribution:

.. code-block:: bash

   % openssl req -x509 -nodes -newkey rsa:2048 -keyout key.pem -out cert.pem -days 365

Note that you need to input the correct ``common name`` when generating the certificate, ``common name`` is typically the domain name or IP of the server.

This will generate two files: ``cert.pem`` and ``key.pem``.  Assign it to the ``-tls`` option when starting overlordd, and you are all set.

.. code-block:: bash

   % nohup ./overlordd -tls=cert.pem,key.pem &

Now you can browse ``https://SERVER_IP:9000`` to access the web frontend.  Note that you need to use ``https`` instead of ``http``.

Connecting to a TLS enabled Server
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

ghost automatically detects if Overlord server has TLS enabled and verify it using system installed ca-certificates bundle.

To connect to a TLS enabled Overlord server with self-signed certificate, a ghost client must specify the TLS certificate to be used for verification:

.. code-block:: bash

   % ghost --tls-cert-file cert.pem SERVER_IP

Optionally, one could enable TLS but skip certificate verification:

.. code-block:: bash

   % ghost --tls-no-verify SERVER_IP

Caveats
~~~~~~~
With a self-signed SSL certificate, the first time you visit the web frontend, you will see a warning about SSL certificate.  This is the result of self-signed SSL certificate, no need to panic.  Click on the left top corner of the browser to see the certificate information.  On Chrome, make sure the fingerprint is correct then hit the ``Advanced`` button then ``Proceed``.

Auto Upgrade Setup
------------------
Overlord supports an AU(Auto Upgrade) protocol for updating ghost clients.  Ghost clients automatically check for update on registration.  Admins can also force an upgrade if there are updates available.

* Prepare Upgrade Files
Fetch the latest ``ghost.py`` or ``ghost`` binary from factory repo.  For the ghost binary, rename it into ghost.ARCH, where ``ARCH`` is go runtime.GOARCH variable on that platform.  For an x86_64 platform, the runtime.GOARCH equals ``amd64``.  In such case, rename the binary to ``ghost.amd64``.

* Create the required directory structure on the server:

.. code-block:: bash

   % mkdir app/upgrade

* Copy the upgrade file

.. code-block:: bash

   % scp ghost.py SERVER_IP:~/overlord/app/upgrade
   % scp ghost.amd64 SERVER_IP:~/overlord/app/upgrade

* Generate Checksum

.. code-block:: bash

   % cd app/upgrade
   % for i in `ls ghost.* | grep -v sha1`; do \
     sha1sum $i | awk '{ print $1 }' > $i.sha1 done

* Force Upgrade
After the above step, the upgrade files are ready.  Now if a new client connects or client reconnects to the overlord server, it automatically checks for the upgrade and apply it.  To force an upgrade for already connected clients, simply send a GET request to the server:

.. code-block:: bash

   % curl -k -u username1:password1 'https://SERVER_IP:9000/api/agents/upgrade'

(Note: use ``http`` if you don't have SSL enabled)

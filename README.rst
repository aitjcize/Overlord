Overlord
========
Overlord is a general purpose device monitoring and proxying framework.

.. image:: https://goreportcard.com/badge/github.com/aitjcize/Overlord
   :target: https://goreportcard.com/report/github.com/aitjcize/Overlord

Overlord provides a web interface which allows you to access device you have control of directly on the web.  Features include:

1. shell access
2. file transfer
3. port forwarding
4. webcam streaming(directly from the web)
5. VPN(to be implemented)

and more.  A CLI tool is also provided for accesing the connected clients.  The Overlord server serve as a proxy, which means the devices are still accessible even if they are behind a NAT.

**Opening terminals directly on the web-based dashboard**

.. image:: https://raw.github.com/aitjcize/Overlord/master/wiki/dashboard.gif

**File upload via drag and drop**

.. image:: https://raw.github.com/aitjcize/Overlord/master/wiki/upload.gif

**File download directly with HTTP download**

.. image:: https://raw.github.com/aitjcize/Overlord/master/wiki/download.gif

Build
-----
Run ``make`` in the project root directory.  ``make`` builds the overlord daemon and the go version of the ghost (overlord client).  The resulting binary can be found in the ``bin`` directory.  Note that You need to have the `Go <https://golang.org/>`_ compiler installed on your system.

Basic Deployment
----------------
1. On a server with public IP (to be used as a proxy), initialize the database by running ``overlordd -init -db-path overlord.db``. You'll be prompted to create an admin username and password.
2. Start the server with ``overlordd -port 9000 -db-path overlord.db`` (if ``-port`` is not specified, default port is 80 or 443 depends on whether TLS is enabled or not).
3. On a client machine, run ``ghost SERVER_IP`` or ``ghost.py SERVER_IP``.  ``ghost`` and ``ghost.py`` are functional equivalent except one is written in Python, and the other is written in Go.
4. Browse http://SERVER_IP:9000 and you will see the overlord web dashboard. Log in with the admin credentials you created during initialization.

For testing purpose, you can run both server and client on the same machine, then browse http://localhost:9000 instead.  Overlord server supports a lot of features such as TLS encryption, client auto upgrade.  For setting up these, please refer to the `Overlord advanced deployment guide <https://github.com/aitjcize/Overlord/blob/master/docs/deployment.rst>`_.

User and Group Management
------------------------
Overlord uses a SQLite database for user and group management. The database is stored in the file specified by the ``-db-path`` parameter (default: ``overlord.db``).

Before starting the server for the first time, you must initialize the database with the ``-init`` flag:

.. code-block:: bash

   overlordd -init -db-path overlord.db

This command will:
1. Create the database schema
2. Generate a secure JWT secret for authentication
3. Prompt you to create an admin user with a custom username and password
4. Create the admin group

You can manage users and groups through the web interface by navigating to:
- Users: ``/users``
- Groups: ``/groups``

Only administrators can create, modify, and delete users and groups.

Allowlist Format
---------------
Overlord supports access control through allowlists that specify which users can access which clients. The allowlist format has been enhanced to support both users and groups with the following prefixes:

- ``u/username`` - Grants access to a specific user
- ``g/groupname`` - Grants access to all members of a specific group
- ``anyone`` - Grants access to all authenticated users

When running a Ghost client, you can specify the allowlist using the ``--allowlist`` parameter:

.. code-block:: bash

   ghost --allowlist u/user1,u/user2,g/group1 SERVER_IP

If no prefix is provided for an entity, it's assumed to be a username and will be automatically prefixed with ``u/``.

Usage
-----
Overlord provides a web interface for interacting with the devices connected to it.  The Overlord server provides a set of APIs that can be used for creating different apps(views) according to the need.  The default dashboard provides an app-switcher on the top right corner.

Besides from the web interface, a command line interface is also provided.  The ``ovl`` command (located at ``scripts/ovl.py``) not only provides the same functionality to the web interface but also provide  command line only functions such as **port forwarding** and **VPN** (to be implemented).  The basic usage is as follows:

1. Connect to the overlord server

.. code-block:: bash

   $ ovl connect SOME_SERVER_IP 9000
   connect: Unauthorized: no authorization request
   Username: user
   Password: 
   connection to SOME_SERVER_IP:9000 established.

2. List connected clients

.. code-block:: bash

   $ ovl ls
   client1
   client2
   client3

3. Select default target to operate on

.. code-block:: bash

   $ ovl select
   Select from the following clients:
       1. client1
       2. client2
       3. client3
   
   Selection: 1

4. Open a shell

.. code-block:: bash

   $ ovl shell
   localhost ~ # _

5. File transfer

.. code-block:: bash

   % ovl push test_file /tmp
   test_file                   9.9 KiB   38.1K/s 00:00 [#####################] 100%
   % ovl pull /tmp/test_file test_file2
   test_file                   9.9 KiB    1.1M/s 00:00 [#####################] 100%

6. Port forwarding: forward the port on client to localhost (assuming we have a web server running on client1's  port 80)

.. code-block:: bash

   % ovl forward 80 9000
   % ovl forward --list
   Client   Remote    Local
   client1  80        9000
   % wget 'http://localhost:9000'
   --2016-03-08 17:56:59--  http://localhost:9000/
   Resolving localhost... ::1, 127.0.0.1
   Connecting to localhost|::1|:9000... failed: Connection refused.
   Connecting to localhost|127.0.0.1|:9000... connected.
   HTTP request sent, awaiting response... 200 OK
   Length: 419 [text/html]
   Saving to: ‘index.html’
   
   index.html          100%[===================>]     419  --.-KB/s    in 0s
   
   2016-03-08 17:57:00 (37.5 MB/s) - ‘index.html’ saved [419/419]

7. User and group management with admin subcommand:

.. code-block:: bash

   # List all users
   % ovl admin list-users
   USERNAME             ADMIN      GROUPS
   --------------------------------------------------
   admin                Yes        admin
   user1                No         testers

   # Add a new user
   % ovl admin add-user username password
   User 'username' created successfully

   # Add user to admin group
   % ovl admin add-user-to-group username admin
   User 'username' added to group 'admin' successfully

   # List groups
   % ovl admin list-groups
   GROUP NAME           USER COUNT
   ------------------------------
   admin                2
   testers              1

   # View users in a group
   % ovl admin list-group-users admin
   Users in group 'admin':
     - admin
     - username

REST API
--------
Overlord provides a REST API for managing users and groups:

- GET /api/users - List all users
- POST /api/users - Create a new user
- DELETE /api/users/{username} - Delete a user
- PUT /api/users/{username}/password - Update a user's password
- GET /api/groups - List all groups
- POST /api/groups - Create a new group
- DELETE /api/groups/{groupname} - Delete a group
- POST /api/groups/{groupname}/users - Add a user to a group
- DELETE /api/groups/{groupname}/users/{username} - Remove a user from a group
- GET /api/groups/{groupname}/users - List all users in a group

Disclaimer
----------
The Overlord project originates from the `Chromium OS factory repository <https://chromium.googlesource.com/chromiumos/platform/factory/>`_, which is used for monitoring and deploying test fixtures in a factory.  The implementation of Overlord is general enough for non-factory use, thus, it's put into this GitHub mirror for greater visibility.  All source code in this repository belongs to the `Chromium OS <https://www.chromium.org/chromium-os>`_ project and the source code is distributed under the same license.

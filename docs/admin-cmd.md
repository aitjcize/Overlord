# Overlord Admin Command Usage Guide

The `ovl` command-line tool includes an `admin` subcommand to manage users and groups in the Overlord system. This guide explains how to use these commands.

## Requirements

- You must be connected to an Overlord server using `ovl connect`
- You must have administrator privileges to use these commands

## User Management

### List Users

View all users in the system:

```bash
ovl admin list-users
```

Example output:
```
USERNAME             ADMIN      GROUPS
--------------------------------------------------
admin                Yes        admin
user1                No         testers
user2                No         testers, developers
```

### Add User

Create a new user:

```bash
ovl admin add-user USERNAME PASSWORD [is_admin]
```

- `USERNAME`: The username for the new user
- `PASSWORD`: The password for the new user
- `is_admin` (optional): Set to "yes", "true", "y", or "1" to make the user an admin

Example:
```bash
# Add a regular user
ovl admin add-user testuser password123

# Add an admin user
ovl admin add-user adminuser password123 yes
```

### Delete User

Remove a user from the system:

```bash
ovl admin del-user USERNAME
```

Example:
```bash
ovl admin del-user testuser
```

### Change Password

Update a user's password:

```bash
ovl admin change-password USERNAME NEW_PASSWORD
```

Example:
```bash
ovl admin change-password testuser newpassword123
```

## Group Management

### List Groups

View all groups in the system:

```bash
ovl admin list-groups
```

Example output:
```
GROUP NAME           USER COUNT
------------------------------
admin                1
testers              2
developers           3
```

### Add Group

Create a new group:

```bash
ovl admin add-group GROUP_NAME
```

Example:
```bash
ovl admin add-group qa-team
```

### Delete Group

Remove a group from the system:

```bash
ovl admin del-group GROUP_NAME
```

Example:
```bash
ovl admin del-group qa-team
```

## User-Group Management

### Add User to Group

Add a user to a group:

```bash
ovl admin add-user-to-group USERNAME GROUP_NAME
```

Example:
```bash
ovl admin add-user-to-group testuser developers
```

### Remove User from Group

Remove a user from a group:

```bash
ovl admin del-user-from-group USERNAME GROUP_NAME
```

Example:
```bash
ovl admin del-user-from-group testuser developers
```

### List Group Users

View all users in a specific group:

```bash
ovl admin list-group-users GROUP_NAME
```

Example:
```bash
ovl admin list-group-users developers
```

Example output:
```
Users in group 'developers':
  - admin
  - user1
  - user2
```

## Examples

### Create a new admin user

```bash
# First create the user
ovl admin add-user newadmin password123

# Then add the user to the admin group
ovl admin add-user-to-group newadmin admin
```

### Set up a team structure

```bash
# Create groups
ovl admin add-group developers
ovl admin add-group qa-team

# Create users
ovl admin add-user dev1 password123
ovl admin add-user dev2 password123
ovl admin add-user qa1 password123

# Assign users to groups
ovl admin add-user-to-group dev1 developers
ovl admin add-user-to-group dev2 developers
ovl admin add-user-to-group qa1 qa-team
``` 
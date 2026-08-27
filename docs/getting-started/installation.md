# Installation

## Installing Gophish-NG Using Pre-Built Binaries

Gophish-NG is provided as a [pre-built binary](https://github.com/raginx/gophish-ng/releases) for most operating systems. With this being the case, installation is as simple as downloading the ZIP file containing the binary that is built for your OS and extracting the contents.

If you're looking for official upstream Gophish binaries instead, see the
[upstream releases page](https://github.com/gophish/gophish/releases/).

## Installing Gophish-NG from Source

One of the major benefits of Gophish being written in the Go programming language is that it is extremely simple to build from source. All you will need is Go 1.25 or above.

```sh
git clone https://github.com/raginx/gophish-ng.git
cd gophish-ng
go build
```

This builds a `gophish` binary in the current directory. The built frontend
assets (`static/js/dist/`, `static/css/dist/`) are checked into git, so this
is all you need for a normal build. If you change anything under
`static/js/src/` or `static/css/`, rebuild them with:

```sh
npm install
npm run build
```

## Understanding the `config.json`

There are some settings that are configurable via a file called config.json, located in the gophish root directory. Here are some of the options that you can set to your preferences:

| Key | Value (Default) | Description |
| :--- | :--- | :--- |
| admin\_server.listen\_url | 127.0.0.1:3333 | IP/Port of gophish admin server |
| admin\_server.use\_tls | false | Use TLS for admin server? |
| admin\_server.cert\_path | example.crt | Path to SSL Cert |
| admin\_server.key\_path | example.key | Path to SSL Private Key |
| admin\_server.trusted_origins | [] | Comma separated list of trusted origins |
| phish\_server.listen\_url | 0.0.0.0:80 | IP/Port of the phishing server - this is where landing pages are hosted. |
| secret\_key | (none) | Base64-encoded 32-byte key used to encrypt OAuth2 tokens at rest (only required if using [OAuth2 IMAP reporting](../automation/email-reporting.md)). Generate one with `openssl rand -base64 32`. |

!!! warning
    **Be careful:** Since the `config.json` file contains database credentials, you will want to ensure it is only readable by the correct user. For Linux users, you can do this using `chmod 640 config.json`.

### Exposing Gophish to the Internet

By default, the `phish_server.listen_url` is configured to listen on all interfaces. This means that if the host Gophish is running on is exposed to the Internet (such as running on a VPS), the phishing server will be exposed to the Internet.

If you also want the admin server to be accessible over the Internet, you will need to change the entry for the `admin_server.listen_url` to `0.0.0.0:3333`.

The `phish_server.trusted_origins` option allows you to add addresses that you expect incoming connections to come from. This is helpful in cases where TLS termination is handled by a load balancer upstream, rather than the application itself.

!!! warning
    **Be careful**: Exposing the admin server to the Internet should only be used if needed. Before exposing the admin server to the Internet, it's **highly recommended** to change the default password.

## Creating SSL Certificate and Private Keys

!!! note
    As of 0.3, Gophish will by default create a self-signed certificate for the admin panel, so this step would be optional.

It's a good idea to have the admin server available over HTTPS. While automatic SSL cert/key generation will be included in a later release, for now let's take a look at how we can leverage openssl to generate our cert and key for use with gophish (this assumes you already have openssl installed!)

We can start the certificate and key generation process with the following command:

```text
openssl req -newkey rsa:2048 -nodes -keyout gophish.key -x509 -days 365 -out gophish.crt
```

Then, all we have to do is answer the CSR process that asks for details such as country, state, etc. Since this is a local self-signed cert, these won’t matter too much to us.

This creates two files, gophish.key and gophish.crt. After moving these files into the gophish root directory (in the same folder as config.json), we can have the following in our config.json file:

```text
    "admin_server" : {
        "listen_url" : "127.0.0.1:3333",
        "use_tls" : true,
        "cert_path" : "gophish.crt",
        "key_path" : "gophish.key"
    }
```

Now when we launch gophish, you’ll connect to the admin server over HTTPS and accept the self-signed certificate warning.

## Using MySQL

The default database in Gophish is SQLite. This is perfectly functional, but some environments may benefit from leveraging a more robust database such as MySQL.

Support for Mysql has been added as of 0.3-dev. To setup Gophish for Mysql, a couple extra steps are needed.

### Update `config.json`

First, change the entries in `config.json` to match your deployment:

Example:

```text
"db_name" : "mysql",
"db_path" : "root:@(:3306)/gophish?charset=utf8&parseTime=True&loc=UTC",
```

The format for the `db_path` entry is `username:password@(host:port)/database?charset=utf8&parseTime=True&loc=UTC`.

### Update MySQL Config

Gophish uses a datetime format that is incompatible with MySQL >= 5.7. To fix this, Add the following lines to the bottom of `/etc/mysql/mysql.cnf`:

```text
[mysqld]
sql_mode=ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION
```

The above settings are the default modes for MySQL, but with NO_ZERO_IN_DATE and NO_ZERO_DATE removed.

### Create the Database

The last step you'll need to do to leverage Mysql is to create the `gophish` database. To do this, log into mysql and run the command

`CREATE DATABASE gophish CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;`.

After that, you'll be good to go!

## Running Gophish

Now that you have gophish installed, you’re ready to run the software. To launch gophish, simply open a command shell and navigate to the directory the gophish binary is located.

Then, execute the gophish binary. You will see some informational output showing both the admin and phishing web servers starting up, as well as the database being created. This output will tell you the port numbers you can use to connect to the web interfaces.

```text
gophish@gophish.dev:~/src/github.com/raginx/gophish-ng$ ./gophish
 time="2020-06-30T08:04:33-05:00" level=warning msg="No contact address has been configured."
 time="2020-06-30T08:04:33-05:00" level=warning msg="Please consider adding a contact_address entry in your config.json"
 time="2020-06-30T08:04:33-05:00" level=info msg="Please login with the username admin and the password 1178f855283d03d3"
 time="2020-06-30T08:04:33-05:00" level=info msg="Starting phishing server at http://0.0.0.0:80"
 time="2020-06-30T08:04:33-05:00" level=info msg="Starting IMAP monitor manager"
 time="2020-06-30T08:04:33-05:00" level=info msg="Starting admin server at https://127.0.0.1:3333"
 time="2020-06-30T08:04:33-05:00" level=info msg="Background Worker Started Successfully - Waiting for Campaigns"
 time="2020-06-30T08:04:33-05:00" level=info msg="Starting new IMAP monitor for user admin"
```

## Health & Version Endpoints

The admin server exposes three unauthenticated `GET` endpoints, useful for
container orchestrators (e.g. Kubernetes liveness/readiness probes) or load
balancer health checks. They're only available on the admin server, not the
phishing server.

| Endpoint | Purpose | Response |
| :--- | :--- | :--- |
| `/healthz` | Liveness probe. Confirms the HTTP server itself is responding - no external dependencies are checked. | `200 OK` — `{"status": "ok"}` |
| `/readyz` | Readiness probe. Verifies the database is reachable before reporting the instance ready to serve traffic. | `200 OK` — `{"status": "ok"}`, or `503 Service Unavailable` — `{"status": "unavailable"}` if the database can't be reached |
| `/version` | Returns the running Gophish-NG version. | `200 OK` — `{"version": "<version>"}` |

!!! note
    These endpoints are intentionally unauthenticated so probes don't need credentials.

## Running Gophish as a Service

### Linux Distributions

To run Gophish as a service in Linux distributions, you will need to setup a service script. You can refer to [this Github issue](https://github.com/gophish/gophish/issues/586) for an example implementation.

### Windows

To run Gophish as a service in Windows, you can use [nssm](http://nssm.cc/).


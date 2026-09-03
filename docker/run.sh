#!/bin/bash

# set config for admin_server
if [ -n "${ADMIN_SERVER_LISTEN_URL+set}" ] ; then
    jq -r \
        --arg ADMIN_SERVER_LISTEN_URL "${ADMIN_SERVER_LISTEN_URL}" \
        '.admin_server.listen_url = $ADMIN_SERVER_LISTEN_URL' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${ADMIN_SERVER_USE_TLS+set}" ] ; then
    jq -r \
        --argjson ADMIN_SERVER_USE_TLS "${ADMIN_SERVER_USE_TLS}" \
        '.admin_server.use_tls = $ADMIN_SERVER_USE_TLS' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${ADMIN_SERVER_CERT_PATH+set}" ] ; then
    jq -r \
        --arg ADMIN_SERVER_CERT_PATH "${ADMIN_SERVER_CERT_PATH}" \
        '.admin_server.cert_path = $ADMIN_SERVER_CERT_PATH' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${ADMIN_SERVER_KEY_PATH+set}" ] ; then
    jq -r \
        --arg ADMIN_SERVER_KEY_PATH "${ADMIN_SERVER_KEY_PATH}" \
        '.admin_server.key_path = $ADMIN_SERVER_KEY_PATH' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${ADMIN_SERVER_TRUSTED_ORIGINS+set}" ] ; then
    jq -r \
        --arg ADMIN_SERVER_TRUSTED_ORIGINS "${ADMIN_SERVER_TRUSTED_ORIGINS}" \
        '.admin_server.trusted_origins = ($ADMIN_SERVER_TRUSTED_ORIGINS|split(","))' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi

# set config for phish_server
if [ -n "${PHISH_SERVER_LISTEN_URL+set}" ] ; then
    jq -r \
        --arg PHISH_SERVER_LISTEN_URL "${PHISH_SERVER_LISTEN_URL}" \
        '.phish_server.listen_url = $PHISH_SERVER_LISTEN_URL' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${PHISH_SERVER_USE_TLS+set}" ] ; then
    jq -r \
        --argjson PHISH_SERVER_USE_TLS "${PHISH_SERVER_USE_TLS}" \
        '.phish_server.use_tls = $PHISH_SERVER_USE_TLS' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${PHISH_SERVER_CERT_PATH+set}" ] ; then
    jq -r \
        --arg PHISH_SERVER_CERT_PATH "${PHISH_SERVER_CERT_PATH}" \
        '.phish_server.cert_path = $PHISH_SERVER_CERT_PATH' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${PHISH_SERVER_KEY_PATH+set}" ] ; then
    jq -r \
        --arg PHISH_SERVER_KEY_PATH "${PHISH_SERVER_KEY_PATH}" \
        '.phish_server.key_path = $PHISH_SERVER_KEY_PATH' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi

# set config for global settings
if [ -n "${DB_NAME+set}" ] ; then
    jq -r \
        --arg DB_NAME "${DB_NAME}" \
        '.db_name = $DB_NAME' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${DB_PATH+set}" ] ; then
    jq -r \
        --arg DB_PATH "${DB_PATH}" \
        '.db_path = $DB_PATH' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${SECRET_KEY+set}" ] ; then
    jq -r \
        --arg SECRET_KEY "${SECRET_KEY}" \
        '.secret_key = $SECRET_KEY' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${MIGRATIONS_PREFIX+set}" ] ; then
    jq -r \
        --arg MIGRATIONS_PREFIX "${MIGRATIONS_PREFIX}" \
        '.migrations_prefix = $MIGRATIONS_PREFIX' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${CONTACT_ADDRESS+set}" ] ; then
    jq -r \
        --arg CONTACT_ADDRESS "${CONTACT_ADDRESS}" \
        '.contact_address = $CONTACT_ADDRESS' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi

# set config for logging
if [ -n "${LOGGING_FILENAME+set}" ] ; then
    jq -r \
        --arg LOGGING_FILENAME "${LOGGING_FILENAME}" \
        '.logging.filename = $LOGGING_FILENAME' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi
if [ -n "${LOGGING_LEVEL+set}" ] ; then
    jq -r \
        --arg LOGGING_LEVEL "${LOGGING_LEVEL}" \
        '.logging.level = $LOGGING_LEVEL' config.json > config.json.tmp && \
        cat config.json.tmp > config.json
fi

echo "Runtime configuration: "
cat config.json

# start gophish
./gophish

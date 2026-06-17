#!/bin/sh
set -e
chown -R xyz:xyz /app/data
exec su-exec xyz /app/analytics

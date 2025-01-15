#!/bin/sh

if [ -f "/etc/dhcpd/conf.yaml" ]; then
    echo "Using mounted config file"
    exec /bin/dhcpd -conf /etc/dhcpd/conf.yaml
else
    echo "Error: No config file found at /etc/dhcpd/conf.yaml"
    exit 1
fi
FROM golang:1.23-alpine

LABEL MAINTAINER="great@linux.com"
LABEL VERSION="v1"

WORKDIR /build

COPY . /build

RUN go build -trimpath -a -o /bin/dhcpd -ldflags="-w -s" cmd/dhcpd/*.go

WORKDIR /etc/dhcpd

RUN mkdir -p /etc/dhcpd
RUN chmod +x /bin/dhcpd

COPY entrypoint.sh /
RUN chmod +x /entrypoint.sh

# server port
EXPOSE 67/udp 
# metrics port
EXPOSE 9100/tcp

ENTRYPOINT ["/entrypoint.sh"]
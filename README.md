## dhcpd

`dhcpd` is a dhcp server written in Go. It is designed to be fast, reliable and easy to configure.

## Install

Get binary from [releases](https://github.com/umegbewe/releases) or clone:

```bash
git clone https://github.com/umegbewe/dhcpd.git
cd dhcpd
```

Build binary:
```
make build
```

## Running dhcpd
Create a conf.yaml with desired settings, see [example conf.yaml](https://raw.githubusercontent.com/umegbewe/dhcpd/main/conf.yaml)

Start dhcp server with configuration

```bash
./dhcpd --conf conf.yaml

INFO[2025-01-03T10:07:47+01:00] [INIT] Set to cleanup expired leases every 120 seconds 
INFO[2025-01-03T10:07:47+01:00] [INIT] Metrics server listening on :9100     
INFO[2025-01-03T10:07:47+01:00] [INIT] DHCP server listening on 0.0.0.0:67 (interface: eth0, IP: 192.168.100.1)
....
```

Test with a client e.g dhcping or dhclient:

The dhcpd is configured to bind to 0.0.0.0 (all interfaces) on port 67 but some client require you to pass an IP address you should pass the interface IP

```
sudo dhcping -v -s 192.168.100.1

Got answer from: 192.168.100.1
```

```
dhclient -i -v eth0

DHCPDISCOVER on eth0 to 255.255.255.255 port 67 interval 3 (xid=0x14e6f62e)
DHCPOFFER of 192.168.100.92 from 192.168.100.1
DHCPREQUEST for 192.168.100.92 on eth0 to 255.255.255.255 port 67 (xid=0x2ef6e614)
DHCPACK of 192.168.100.92 from 192.168.100.1 (xid=0x14e6f62e)
bound to 192.168.100.92 -- renewal in 261 seconds.
```

dhcpd uses boltdb to maintain lease data across restarts or crashes, there is a potential for support swapping lease persistent backends in the future (leases.txt, redis, mysql, postgres etc)

Optional integration monitoring dhcp operations and performance metrics, see ``:9100/metrics``, also a sample grafana dashboard json to visualize those metrics



<img src="https://github.com/umegbewe/dhcpd/blob/main/grafana/screenshot.png">

## Future work
At this stage dhcpd will work well for development environments and small networks.

* Subnet/scope configuration
* DHCP relay support
* Vendor specific options
* Optional embedded web UI/API for managing leases
* Support other databases for lease persistence
* Choose allocation straetgies i.e Random, Sequential or Segment (current: sequential)







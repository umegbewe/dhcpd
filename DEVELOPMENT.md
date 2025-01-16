# Development

This doc will guide you through testing ``dhcpd`` safely without impacting your main network. Deploying a DHCP server on your main network can cause conflicts with existing DHCP services (like the one provided by your router). 

We'll set up an isolated test environment using linux network namespaces and virtual Ethernet interfaces.


## Guide

create network namespaces for client and server

```bash
sudo ip netns add server_ns
sudo ip netns add client_ns
```

create virtual ethernet pair to connect server and client namespace

```bash
sudo ip link add veth-server type veth peer name veth-client
```

assign interfaces to namespaces
```bash
sudo ip link set veth-server netns server_ns
sudo ip link set veth-client netns client_ns
```

configure server namespace

```bash
sudo ip netns exec server_ns ip addr add 192.168.100.1/24 dev veth-server
sudo ip netns exec server_ns ip link set dev veth-server up
sudo ip netns exec server_ns ip link set dev lo up
sudo ip netns exec server_ns ip route add 255.255.255.255 dev veth-server

```


configure client namespace
```bash 
sudo ip netns exec client_ns ip link set dev veth-client up
sudo ip netns exec client_ns ip link set dev lo up
```

update config to listen on ``veth server``

conf.yaml:
```yaml
server:
  ip_start: 192.168.100.10
  ip_end: 192.168.100.255
  subnet_mask: 255.255.255.0
  lease_time: 600
  gateway: 192.168.1.10
  server_ip: 192.168.1.10
  port: 67
  dns_servers:
    - 8.8.8.8
    - 8.8.4.4
  interface: en0
  lease_db_path: leases.db
  cleanup_expired_interval: 120
logging:
  level: info
metrics:
  enabled: true
  listen_address: '0.0.0.0:9100' 
```

run dhcpd in server namespace

```bash
sudo ip netns exec server_ns ./dhcpd
```
ensure it's running and listening on ``veth-server``, you should see an output similar to this:

```
INFO[2025-01-03T10:07:47+01:00] [INIT] Set to cleanup expired leases every 120 seconds 
INFO[2025-01-03T10:07:47+01:00] [INIT] Metrics server listening on :9100     
INFO[2025-01-03T10:07:47+01:00] [INIT] DHCP server listening on 0.0.0.0:67 (interface: veth-server, IP: 192.168.100.10)
```


run dhcp client in client namespace

```bash
sudo ip netns exec client_ns dhclient -v veth-client
```

verify ip assignment 
```bash
sudo ip netns exec client_ns ip addr show dev veth-client
```

you should see an IP assigned within the range specified in ``conf.yaml``

```bash
3: veth-client@if4: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP 
    link/ether ba:ad:c0:ff:ee:01 brd ff:ff:ff:ff:ff:ff
    inet 192.168.100.10/24 brd 192.168.100.255 scope global dynamic veth-client
       valid_lft 86396sec preferred_lft 86396sec
```

cleanup

```bash
sudo ip netns del server_ns
sudo ip netns del client_ns
sudo ip link delete veth-server
```
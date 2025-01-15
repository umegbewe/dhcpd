package dhcp

import (
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/umegbewe/dhcpd/internal/config"
	"github.com/umegbewe/dhcpd/pkg/dhcp"
)

func createTestConfig(dbPath string) *config.Config {
	return &config.Config{
		Server: struct {
			IPStart                string   `yaml:"ip_start"`
			IPEnd                  string   `yaml:"ip_end"`
			SubnetMask             string   `yaml:"subnet_mask"`
			LeaseTime              int      `yaml:"lease_time"`
			Gateway                string   `yaml:"gateway"`
			ServerIP               string   `yaml:"server_ip"`
			DNSServers             []string `yaml:"dns_servers"`
			TFTPServerName         string   `yaml:"tftp_server_name"`
			BootFileName           string   `yaml:"boot_file_name"`
			Interface              string   `yaml:"interface"`
			Port                   int      `yaml:"port" default:"67"`
			LeaseDBPath            string   `yaml:"lease_db_path"`
			CleanupExpiredInterval int      `yaml:"cleanup_expired_interval" default:"120"`
		}{
			IPStart:     "192.168.100.10",
			IPEnd:       "192.168.100.12",
			SubnetMask:  "255.255.255.0",
			LeaseTime:   60,
			Gateway:     "192.168.100.1",
			ServerIP:    "192.168.100.1",
			DNSServers:  []string{"8.8.8.8", "8.8.4.4"},
			Interface:   "eth0",
			Port:        6767,
			LeaseDBPath: dbPath,
		},
	}
}

func TestDHCPMessageExchange(t *testing.T) {

	cfg := createTestConfig("test_message_exchange.db")
	pool, err := dhcp.LeasePool(cfg.Server.IPStart, cfg.Server.IPEnd, cfg.Server.LeaseTime, cfg.Server.LeaseDBPath)
	if err != nil {
		t.Fatalf("Failed to create lease pool: %v", err)
	}
	defer pool.Close()

	iface, err := net.InterfaceByName(cfg.Server.Interface)
	if err != nil {
		t.Fatalf("Failed to get network interface: %v", err)
	}

	srv := &dhcp.Server{
		Config:    cfg,
		LeasePool: pool,
		Interface: iface,
	}

	defer os.Remove(cfg.Server.LeaseDBPath)

	srv.Connection, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatalf("Failed to create dummy UDP connection: %v", err)
	}
	defer srv.Connection.Close()

	msg := dhcp.NewMessage()
	msg.OpCode = 1
	msg.CHAddr = net.HardwareAddr{0x00, 0xAB, 0xCD, 0x12, 0x34, 0x56}
	msg.Options.Add(dhcp.OptionDHCPMessageType, uint8(dhcp.MessageTypeDiscover))

	srv.HandleDiscover(msg, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 68})

	l := pool.GetLeaseByMAC(msg.CHAddr)
	if l == nil {
		t.Fatalf("Expected to find a lease for MAC=%s, but found none", msg.CHAddr)
	}
}

func TestConcurrentDiscovers(t *testing.T) {
	cfg := createTestConfig("test_message_exchange.db")

	pool, err := dhcp.LeasePool(cfg.Server.IPStart, cfg.Server.IPEnd, cfg.Server.LeaseTime, cfg.Server.LeaseDBPath)
	if err != nil {
		t.Fatalf("Failed to create lease pool: %v", err)
	}
	defer pool.Close()

	iface, err := net.InterfaceByName(cfg.Server.Interface)
	if err != nil {
		t.Fatalf("Failed to get network interface: %v", err)
	}

	srv := &dhcp.Server{
		Config:    cfg,
		LeasePool: pool,
		Interface: iface,
	}

	defer os.Remove(cfg.Server.LeaseDBPath)

	srv.Connection, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatalf("Failed to create dummy UDP connection: %v", err)
	}
	defer srv.Connection.Close()

	var wg sync.WaitGroup
	base := []byte{0x00, 0x00, 0x00, 0x12, 0x34, 0x00}
	discovers := 5

	for i := 0; i < discovers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mac := append([]byte(nil), base...)
			mac[5] = byte(idx)
			msg := dhcp.NewMessage()
			msg.OpCode = 1
			msg.CHAddr = net.HardwareAddr(mac)
			msg.Options.Add(dhcp.OptionDHCPMessageType, uint8(dhcp.MessageTypeDiscover))

			srv.HandleDiscover(msg, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 68})
		}(i)
	}
	wg.Wait()

	allocated := 0
	for i := 0; i < discovers; i++ {
		mac := append([]byte(nil), base...)
		mac[5] = byte(i)
		if l := pool.GetLeaseByMAC(mac); l != nil {
			allocated++
		}
	}

	if allocated > 3 {
		t.Errorf("Expected max 3 allocated leases, got=%d", allocated)
	}
}

func TestRequestFlow(t *testing.T) {
	cfg := &config.Config{
		Server: struct {
			IPStart                string   `yaml:"ip_start"`
			IPEnd                  string   `yaml:"ip_end"`
			SubnetMask             string   `yaml:"subnet_mask"`
			LeaseTime              int      `yaml:"lease_time"`
			Gateway                string   `yaml:"gateway"`
			ServerIP               string   `yaml:"server_ip"`
			DNSServers             []string `yaml:"dns_servers"`
			TFTPServerName         string   `yaml:"tftp_server_name"`
			BootFileName           string   `yaml:"boot_file_name"`
			Interface              string   `yaml:"interface"`
			Port                   int      `yaml:"port" default:"67"`
			LeaseDBPath            string   `yaml:"lease_db_path"`
			CleanupExpiredInterval int      `yaml:"cleanup_expired_interval" default:"120"`
		}{
			IPStart:     "192.168.100.10",
			IPEnd:       "192.168.100.15",
			SubnetMask:  "255.255.255.0",
			LeaseTime:   60,
			Gateway:     "192.168.100.1",
			ServerIP:    "192.168.100.1",
			DNSServers:  []string{"8.8.8.8"},
			Interface:   "eth0",
			Port:        6767,
			LeaseDBPath: "test_request_flow.db",
		},
	}

	pool, err := dhcp.LeasePool(cfg.Server.IPStart, cfg.Server.IPEnd, cfg.Server.LeaseTime, cfg.Server.LeaseDBPath)
	if err != nil {
		t.Fatalf("Failed to create lease pool: %v", err)
	}
	defer pool.Close()

	iface, err := net.InterfaceByName(cfg.Server.Interface)
	if err != nil {
		t.Fatalf("Failed to get network interface: %v", err)
	}

	srv := &dhcp.Server{
		Config:    cfg,
		LeasePool: pool,
		Interface: iface,
	}
	defer os.Remove(cfg.Server.LeaseDBPath)

	srv.Connection, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatalf("Failed to create dummy UDP connection: %v", err)
	}
	defer srv.Connection.Close()

	// simulate DISCOVER
	discover := dhcp.NewMessage()
	discover.OpCode = 1
	discover.CHAddr = net.HardwareAddr{0x00, 0xAB, 0xCD, 0x12, 0x34, 0x56}
	discover.Options.Add(dhcp.OptionDHCPMessageType, uint8(dhcp.MessageTypeDiscover))

	srv.HandleDiscover(discover, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 68})
	leaseRec := pool.GetLeaseByMAC(discover.CHAddr)
	if leaseRec == nil {
		t.Fatalf("Expected an allocated lease for MAC=%s, but found none", discover.CHAddr)
	}

	request := dhcp.NewMessage()
	request.OpCode = 1
	request.CHAddr = discover.CHAddr
	request.Options.Add(dhcp.OptionDHCPMessageType, uint8(dhcp.MessageTypeRequest))
	requestedIP := leaseRec.IP.To4()
	request.Options.Add(dhcp.OptionRequestedIPAddress, requestedIP)

	srv.HandleRequest(request, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 68})

	// verify lease is still allocated (ACK)
	if rec := pool.GetLeaseByMAC(discover.CHAddr); rec == nil {
		t.Error("Client lease not found after REQUEST, expected ACK allocation to remain")
	} else if !rec.IP.Equal(requestedIP) {
		t.Errorf("Expected IP=%s, got IP=%s for MAC=%s", requestedIP, rec.IP, discover.CHAddr)
	}
}

func TestHighConcurrencyFlood(t *testing.T) {
	cfg := &config.Config{
		Server: struct {
			IPStart                string   `yaml:"ip_start"`
			IPEnd                  string   `yaml:"ip_end"`
			SubnetMask             string   `yaml:"subnet_mask"`
			LeaseTime              int      `yaml:"lease_time"`
			Gateway                string   `yaml:"gateway"`
			ServerIP               string   `yaml:"server_ip"`
			DNSServers             []string `yaml:"dns_servers"`
			TFTPServerName         string   `yaml:"tftp_server_name"`
			BootFileName           string   `yaml:"boot_file_name"`
			Interface              string   `yaml:"interface"`
			Port                   int      `yaml:"port" default:"67"`
			LeaseDBPath            string   `yaml:"lease_db_path"`
			CleanupExpiredInterval int      `yaml:"cleanup_expired_interval" default:"120"`
		}{
			IPStart:     "192.168.0.10",
			IPEnd:       "192.183.255.0",
			SubnetMask:  "255.240.0.0",
			LeaseTime:   2000,
			Gateway:     "192.168.200.1",
			ServerIP:    "192.168.200.1",
			DNSServers:  []string{"8.8.8.8", "8.8.4.4"},
			Interface:   "eth0",
			Port:        6767,
			LeaseDBPath: "test_high_concurrency_flood.db",
		}, Logging: struct {
			Level string `yaml:"level"`
		}{Level: "error"},
	}

	pool, err := dhcp.LeasePool(cfg.Server.IPStart, cfg.Server.IPEnd, cfg.Server.LeaseTime, cfg.Server.LeaseDBPath)
	if err != nil {
		t.Fatalf("Failed to create lease pool: %v", err)
	}

	iface, err := net.InterfaceByName(cfg.Server.Interface)
	if err != nil {
		t.Fatalf("Failed to get network interface: %v", err)
	}

	srv := &dhcp.Server{
		Config:    cfg,
		LeasePool: pool,
		Interface: iface,
	}

	defer os.Remove(cfg.Server.LeaseDBPath)

	srv.Connection, err = net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatalf("Failed to create dummy UDP connection: %v", err)
	}
	defer srv.Connection.Close()
	defer srv.LeasePool.Close()

	const concurrencyLevel = 20
	const messagesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(concurrencyLevel)

	for i := 0; i < concurrencyLevel; i++ {
		go func(gid int) {
			defer wg.Done()

			for j := 0; j < messagesPerGoroutine; j++ {
				mac := net.HardwareAddr{0x00, 0xAB, 0xCD, byte(gid >> 8), byte(gid), byte(j)}

				discover := dhcp.NewMessage()
				discover.OpCode = 1
				discover.CHAddr = mac
				discover.Options.Add(dhcp.OptionDHCPMessageType, uint8(dhcp.MessageTypeDiscover))

				srv.HandleDiscover(discover, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 68})

				allocated := srv.LeasePool.GetLeaseByMAC(mac)
				if allocated == nil {
					t.Errorf("No lease allocated for MAC=%s in goroutine %d, attempt %d", mac, gid, j)
					continue
				}

				request := dhcp.NewMessage()
				request.OpCode = 1
				request.CHAddr = mac
				request.Options.Add(dhcp.OptionDHCPMessageType, uint8(dhcp.MessageTypeRequest))
				request.Options.Add(dhcp.OptionRequestedIPAddress, allocated.IP.To4())

				srv.HandleRequest(request, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 68})

				ackLease := srv.LeasePool.GetLeaseByMAC(mac)
				if ackLease == nil {
					t.Errorf("Lease lost after REQUEST for MAC=%s in goroutine %d, attempt %d", mac, gid, j)
					continue
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	leasedCount := 0
	for _, r := range srv.LeasePool.ListLeases() {
		if r != nil {
			leasedCount++
		}
	}

	if leasedCount == 0 {
		t.Error("Expected at least some leases to be allocated, found none.")
	}
}

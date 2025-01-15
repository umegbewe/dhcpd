package dhcp

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/umegbewe/dhcpd/pkg/dhcp"
)

func TestLeasePoolBasic(t *testing.T) {
	dbFile := "test_leases_pool.db"
	defer os.Remove(dbFile)

	pool, err := dhcp.LeasePool("192.168.100.10", "192.168.100.20", 60, dbFile)
	if err != nil {
		t.Fatalf("Failed to create lease pool: %v", err)
	}
	defer pool.Close()

	mac := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	ip, err := pool.Allocate(&net.Interface{Name: "eth0"}, mac)
	if err != nil {
		t.Fatalf("Failed to allocate IP for MAC=%s: %v", mac, err)
	}

	leaseRec := pool.GetLeaseByMAC(mac)
	if leaseRec == nil {
		t.Fatalf("Lease not found for MAC=%s after allocation", mac)
	}
	if !leaseRec.IP.Equal(ip) {
		t.Errorf("Expected IP=%s, got=%s", ip, leaseRec.IP)
	}

	err = pool.Release(ip)
	if err != nil {
		t.Errorf("Failed to release IP=%s: %v", ip, err)
	}

	leaseRec = pool.GetLeaseByMAC(mac)
	if leaseRec != nil {
		t.Errorf("Expected no lease for MAC=%s after release, got %v", mac, leaseRec)
	}
}

func TestLeasePoolExhaustion(t *testing.T) {
	dbFile := "test_leases_exhaust.db"
	defer os.Remove(dbFile)

	pool, err := dhcp.LeasePool("192.168.100.10", "192.168.100.12", 60, dbFile)
	if err != nil {
		t.Fatalf("Failed to create lease pool: %v", err)
	}
	defer pool.Close()

	// enough MACs to exhaust the range 192.168.100.10-192.168.100.12
	macs := []net.HardwareAddr{
		{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x10},
		{0xDE, 0xAD, 0xBE, 0xEF, 0x02, 0x20},
		{0xDE, 0xAD, 0xBE, 0xEF, 0x03, 0x30},
	}
	for _, mac := range macs {
		_, err := pool.Allocate(&net.Interface{Name: "eth0"}, mac)
		if err != nil {
			t.Fatalf("Unexpected error allocating IP: %v", err)
		}
	}

	// next allocation should fail
	_, err = pool.Allocate(&net.Interface{Name: "eth0"}, net.HardwareAddr{0xDE, 0xAD, 0xBE, 0xEF, 0x04, 0x40})
	if err == nil {
		t.Fatal("Expected an error due to IP exhaustion, but got nil")
	}
}

func TestLeasePoolExpiry(t *testing.T) {
	dbFile := "test_leases_expiry.db"
	defer os.Remove(dbFile)

	pool, err := dhcp.LeasePool("192.168.100.10", "192.168.100.10", 1, dbFile)
	if err != nil {
		t.Fatalf("Failed to create lease pool: %v", err)
	}
	defer pool.Close()

	mac := net.HardwareAddr{0xCA, 0xFE, 0xBA, 0xBE, 0xBC, 0x01}
	ip, err := pool.Allocate(&net.Interface{Name: "eth0"}, mac)
	if err != nil {
		t.Fatalf("Failed to allocate IP=%v", err)
	}

	time.Sleep(2 * time.Second)

	leaseRec := pool.GetLeaseByMAC(mac)
	if leaseRec != nil {
		t.Fatalf("Expected lease to be expired and removed, but found %v", leaseRec)
	}

	// allocation should return the same IP
	ip2, err := pool.Allocate(&net.Interface{Name: "eth0"}, mac)
	if err != nil {
		t.Fatalf("Failed to re-allocate IP after expiry: %v", err)
	}
	if !ip2.Equal(ip) {
		t.Errorf("Expected IP=%s after re-allocation, got=%s", ip, ip2)
	}
}

func TestLeaseWrapAround(t *testing.T) {
	dbFile := "test_lease_wraparound.db"
	defer os.Remove(dbFile)

	pool, err := dhcp.LeasePool("192.168.0.100", "192.168.0.104", 1, dbFile)
	if err != nil {
		t.Fatalf("Failed to create lease pool: %v", err)
	}
	defer pool.Close()

	iface := &net.Interface{Name: "eth0"}

	mac1, _ := net.ParseMAC("00:11:22:33:44:01")
	mac2, _ := net.ParseMAC("00:11:22:33:44:02")
	mac3, _ := net.ParseMAC("00:11:22:33:44:03")
	mac4, _ := net.ParseMAC("00:11:22:33:44:04")

	//(offset 0)
	ip1, err := pool.Allocate(iface, mac1)
	if err != nil {
		t.Fatalf("Allocate(mac1) failed: %v", err)
	}
	if ip1.String() != "192.168.0.100" {
		t.Errorf("Expected 192.168.0.100 for mac1, got %s", ip1)
	}

	//(offset 1)
	ip2, err := pool.Allocate(iface, mac2)
	if err != nil {
		t.Fatalf("Allocate(mac2) failed: %v", err)
	}
	if ip2.String() != "192.168.0.101" {
		t.Errorf("Expected 192.168.0.101 for mac2, got %s", ip2)
	}

	//(offset 2)
	ip3, err := pool.Allocate(iface, mac3)
	if err != nil {
		t.Fatalf("Allocate(mac3) failed: %v", err)
	}
	if ip3.String() != "192.168.0.102" {
		t.Errorf("Expected 192.168.0.102 for mac3, got %s", ip3)
	}

	// Release mac2, so offset 1 becomes free
	if err := pool.Release(ip2); err != nil {
		t.Fatalf("Release(ip2) failed: %v", err)
	}

	mac4IP, err := pool.Allocate(iface, mac4)
	if err != nil {
		t.Fatalf("Allocate(mac4) failed: %v", err)
	}
	if mac4IP.String() != "192.168.0.103" {
		t.Errorf("Expected 192.168.0.103 for mac4, got %s", mac4IP)
	}

	mac5, _ := net.ParseMAC("00:11:22:33:44:05")
	ip5, err := pool.Allocate(iface, mac5)
	if err != nil {
		t.Fatalf("Allocate(mac5) failed: %v", err)
	}
	if ip5.String() != "192.168.0.104" {
		t.Errorf("Expected 192.168.0.104 for mac5, got %s", ip5)
	}

	mac6, _ := net.ParseMAC("00:11:22:33:44:06")
	ip6, err := pool.Allocate(iface, mac6)
	if err != nil {
		t.Fatalf("Allocate(mac6) failed: %v", err)
	}
	if ip6.String() != "192.168.0.101" {
		t.Errorf("Expected wrap-around to 192.168.0.101, got %s", ip6)
	}
}

package dhcp

import (
	"bytes"
	"net"
	"testing"

	"github.com/umegbewe/dhcpd/pkg/dhcp"
)

func TestOptionsEncodeDecode(t *testing.T) {
	var opts dhcp.Options
	opts.Add(dhcp.OptionDHCPMessageType, uint8(dhcp.MessageTypeDiscover))
	opts.Add(dhcp.OptionSubnetMask, net.ParseIP("255.255.255.0"))
	opts.Add(dhcp.OptionRouterAddress, net.ParseIP("192.168.0.1"))

	encoded, err := opts.Encode()
	if err != nil {
		t.Fatalf("Failed to encode options: %v", err)
	}

	decodedOpts, err := dhcp.DecodeOptions(encoded)
	if err != nil {
		t.Fatalf("Failed to decode options: %v", err)
	}

	if len(decodedOpts) != 3 {
		t.Errorf("Expected 3 options, got=%d", len(decodedOpts))
	}

	mType := uint8(0)
	var subnet, router net.IP
	for _, opt := range decodedOpts {
		switch opt.Code {
		case dhcp.OptionDHCPMessageType:
			mType = opt.Value[0]
		case dhcp.OptionSubnetMask:
			subnet = net.IP(opt.Value)
		case dhcp.OptionRouterAddress:
			router = net.IP(opt.Value)
		}
	}

	if mType != dhcp.MessageTypeDiscover {
		t.Errorf("Expected message type=%d, got=%d", dhcp.MessageTypeDiscover, mType)
	}
	if !subnet.Equal(net.IPv4(255, 255, 255, 0)) {
		t.Errorf("Expected subnet mask=255.255.255.0, got=%v", subnet)
	}
	if !router.Equal(net.IPv4(192, 168, 0, 1)) {
		t.Errorf("Expected router=192.168.0.1, got=%v", router)
	}
}

func TestOptionsAddMultipleDNS(t *testing.T) {
	var opts dhcp.Options
	opts.Add(dhcp.OptionDNSServers, []net.IP{
		net.IPv4(8, 8, 8, 8),
		net.IPv4(8, 8, 4, 4),
	})

	encoded, err := opts.Encode()
	if err != nil {
		t.Fatalf("Failed to encode options: %v", err)
	}

	decoded, err := dhcp.DecodeOptions(encoded)
	if err != nil {
		t.Fatalf("Could not decode multiple DNS options: %v", err)
	}

	if len(decoded) != 1 {
		t.Fatalf("Expected 1 option, got=%d", len(decoded))
	}

	if decoded[0].Code != dhcp.OptionDNSServers {
		t.Fatalf("Expected code=OptionDNSServers, got=%d", decoded[0].Code)
	}

	if len(decoded[0].Value) != 8 {
		t.Fatalf("Expected 8 bytes for two DNSServers, got=%d", len(decoded[0].Value))
	}

	if !bytes.Equal(decoded[0].Value[:4], net.IPv4(8, 8, 8, 8).To4()) ||
		!bytes.Equal(decoded[0].Value[4:], net.IPv4(8, 8, 4, 4).To4()) {
		t.Fatalf("DNS IPs do not match expected values: %v", decoded[0].Value)
	}
}

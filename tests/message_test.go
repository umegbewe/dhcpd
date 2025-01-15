package dhcp

import (
	"net"
	"testing"

	"github.com/umegbewe/dhcpd/pkg/dhcp"
)

func TestMessageEncodeDecode(t *testing.T) {
	msg := dhcp.NewMessage()
	msg.XID = 0xABCD1234
	msg.CIAddr = net.IPv4(192, 168, 0, 10)
	msg.Options.Add(dhcp.OptionDHCPMessageType, uint8(dhcp.MessageTypeDiscover))

	encoded, err := msg.Encode()
	if err != nil {
		t.Errorf("Encoded DHCP message failed %v", err)
	}

	if len(encoded) < 240 {
		t.Errorf("Encoded DHCP message is too short: %d bytes", len(encoded))
	}

	decoded, err := dhcp.DecodeMessage(encoded)
	if err != nil {
		t.Fatalf("Failed to decode message: %v", err)
	}

	if decoded.XID != msg.XID {
		t.Errorf("Expected XID=%08x, got=%08x", msg.XID, decoded.XID)
	}
	if !decoded.CIAddr.Equal(msg.CIAddr) {
		t.Errorf("Expected CIAddr=%s, got=%s", msg.CIAddr, decoded.CIAddr)
	}

	expectedType := msg.Options.GetMessageType()
	gotType := decoded.Options.GetMessageType()
	if gotType != expectedType {
		t.Errorf("Expected DHCPMessageType=%d, got=%d", expectedType, gotType)
	}
}

func TestInvalidMessageData(t *testing.T) {
	shortData := []byte{0x01, 0x01, 0x06}
	if _, err := dhcp.DecodeMessage(shortData); err == nil {
		t.Error("Expected error when decoding short data, but got nil")
	}

	data := make([]byte, 240)
	if _, err := dhcp.DecodeMessage(data); err == nil {
		t.Error("Expected error due to missing magic cookie, but got nil")
	}
}

func TestMessageOptions(t *testing.T) {
	msg := dhcp.NewMessage()
	msg.Options.Add(dhcp.OptionDHCPMessageType, uint8(dhcp.MessageTypeRequest))
	msg.Options.Add(dhcp.OptionRequestedIPAddress, net.IPv4(192, 168, 0, 50))

	if msg.Options.GetMessageType() != dhcp.MessageTypeRequest {
		t.Errorf("Expected message type=%d, got=%d", dhcp.MessageTypeRequest, msg.Options.GetMessageType())
	}

	reqIP := msg.Options.GetRequestedIP()
	if reqIP == nil || !reqIP.Equal(net.IPv4(192, 168, 0, 50)) {
		t.Errorf("Expected requested IP=192.168.0.50, got=%v", reqIP)
	}
}

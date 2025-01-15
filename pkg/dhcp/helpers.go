package dhcp

import (
	"encoding/binary"
	"net"
)


func ipToUint32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip.To4())
}

func offsetToIP(base net.IP, offset int) net.IP {
    baseInt := ipToUint32(base)
    newInt := baseInt + uint32(offset)
    ipBytes := make([]byte, 4)
    binary.BigEndian.PutUint32(ipBytes, newInt)


    return net.IP(ipBytes)
}


func messageTypeToString(msgType uint8) string {
	switch msgType {
	case MessageTypeDiscover:
		return "discover"
	case MessageTypeOffer:
		return "offer"
	case MessageTypeRequest:
		return "request"
	case MessageTypeDecline:
		return "decline"
	case MessageTypeAck:
		return "ack"
	case MessageTypeNak:
		return "nak"
	case MessageTypeRelease:
		return "release"
	case MessageTypeInform:
		return "inform"
	default:
		return "unknown"
	}
}

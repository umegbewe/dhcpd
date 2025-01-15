package dhcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

const (
	MessageTypeDiscover = 1
	MessageTypeOffer    = 2
	MessageTypeRequest  = 3
	MessageTypeDecline  = 4
	MessageTypeAck      = 5
	MessageTypeNak      = 6
	MessageTypeRelease  = 7
	MessageTypeInform   = 8
)

var MagicCookie = []byte{0x63, 0x82, 0x53, 0x63}

type Message struct {
	OpCode  uint8
	HType   uint8
	HLen    uint8
	Hops    uint8
	XID     uint32
	Secs    uint16
	Flags   uint16
	CIAddr  net.IP
	YIAddr  net.IP
	SIAddr  net.IP
	GIAddr  net.IP
	CHAddr  net.HardwareAddr
	SName   [64]byte
	File    [128]byte
	Options Options
}

func NewMessage() *Message {
	return &Message{
		OpCode:  2,
		HType:   1,                // ethernet
		HLen:    6,                // MAC address length
		CHAddr:  make([]byte, 16), // client hardware address
		Options: make(Options, 0),
	}
}

func (m *Message) Encode() ([]byte, error) {
	var buf bytes.Buffer
	write := func(value interface{}) error {
		return binary.Write(&buf, binary.BigEndian, value)
	}

	if err := write(m.OpCode); err != nil {
		return nil, err
	}
	if err := write(m.HType); err != nil {
		return nil, err
	}
	if err := write(m.HLen); err != nil {
		return nil, err
	}
	if err := write(m.Hops); err != nil {
		return nil, err
	}
	if err := write(m.XID); err != nil {
		return nil, err
	}
	if err := write(m.Secs); err != nil {
		return nil, err
	}
	if err := write(m.Flags); err != nil {
		return nil, err
	}

	writeIP4(&buf, m.CIAddr)
	writeIP4(&buf, m.YIAddr)
	writeIP4(&buf, m.SIAddr)
	writeIP4(&buf, m.GIAddr)

	for _, slice := range [][]byte{m.CHAddr, m.SName[:], m.File[:]} {
		if _, err := buf.Write(slice); err != nil {
			return nil, err
		}
	}

	if _, err := buf.Write(MagicCookie); err != nil {
		return nil, err
	}

	options, err := m.Options.Encode()
	if err != nil {
		return nil, err
	}
	if _, err := buf.Write(options); err != nil {
		return nil, err
	}
	
	return buf.Bytes(), nil
}

func DecodeMessage(data []byte) (*Message, error) {
	if len(data) < 240 {
		return nil, errors.New("DHCP message too short")
	}

	if !bytes.Equal(data[236:240], MagicCookie) {
		return nil, errors.New("invalid or missing DHCP magic cookie")
	}

	var m Message
	buf := bytes.NewReader(data)

	read := func(value interface{}) error {
		return binary.Read(buf, binary.BigEndian, value)
	}


	if err := read(&m.OpCode); err != nil {
		return nil, err
	}
	if err := read(&m.HType); err != nil {
		return nil, err
	}
	if err := read(&m.HLen); err != nil {
		return nil, err
	}
	if err := read(&m.Hops); err != nil {
		return nil, err
	}
	if err := read(&m.XID); err != nil {
		return nil, err
	}
	if err := read(&m.Secs); err != nil {
		return nil, err
	}
	if err := read(&m.Flags); err != nil {
		return nil, err
	}

	ci := make([]byte, 4)
	yi := make([]byte, 4)
	si := make([]byte, 4)
	gi := make([]byte, 4)
	if _, err := buf.Read(ci); err != nil {
		return nil, err
	}
	if _, err := buf.Read(yi); err != nil {
		return nil, err
	}
	if _, err := buf.Read(si); err != nil {
		return nil, err
	}
	if _, err := buf.Read(gi); err != nil {
		return nil, err
	}
	m.CIAddr = net.IP(ci)
	m.YIAddr = net.IP(yi)
	m.SIAddr = net.IP(si)
	m.GIAddr = net.IP(gi)

	chaddr := make([]byte, 16)
	if _, err := buf.Read(chaddr); err != nil {
		return nil, err
	}
	if m.HLen > 16 {
		return nil, fmt.Errorf("invalid hardware address length: %d", m.HLen)
	}
	m.CHAddr = make(net.HardwareAddr, m.HLen)
	copy(m.CHAddr, chaddr[:m.HLen])

	if _, err := buf.Read(m.SName[:]); err != nil {
		return nil, err
	}
	if _, err := buf.Read(m.File[:]); err != nil {
		return nil, err
	}

	magic := make([]byte, 4)
	if _, err := buf.Read(magic); err != nil {
		return nil, err
	}

	optionsData := data[240:]
	options, err := DecodeOptions(optionsData)
	if err != nil {
		return nil, err
	}

	m.Options = options

	return &m, nil
}

func writeIP4(buf *bytes.Buffer, ip net.IP) {
	if ip4 := ip.To4(); ip4 != nil {
		buf.Write(ip4)
	} else {
		buf.Write([]byte{0, 0, 0, 0})
	}
}

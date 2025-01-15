package dhcp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"

	log "github.com/sirupsen/logrus"
)

type OptionCode uint8

const (
	OptionPad                   OptionCode = 0
	OptionSubnetMask            OptionCode = 1
	OptionRouterAddress         OptionCode = 3
	OptionDNSServers            OptionCode = 6
	OptionHostName              OptionCode = 12
	OptionRequestedIPAddress    OptionCode = 50
	OptionIPAddressLeaseTime    OptionCode = 51
	OptionDHCPMessageType       OptionCode = 53
	OptionServerIdentifier      OptionCode = 54
	OptionParameterRequestList  OptionCode = 55
	OptionVendorClassIdentifier OptionCode = 60
	OptionClientIdentifier      OptionCode = 61
	OptionTFTPServerName        OptionCode = 66
	OptionBootfileName          OptionCode = 67
	OptionEndOption             OptionCode = 255
)

type Option struct {
	Code  OptionCode
	Value []byte
}

type Options []Option

func (o Options) Encode() ([]byte, error) {
	var buf bytes.Buffer
	write := func(value interface{}) error {
		return binary.Write(&buf, binary.BigEndian, value)
	}

	for _, opt := range o {
		if opt.Code == OptionEndOption {
			continue
		}

		if err := write(opt.Code); err != nil {
			return nil, err
		}

		if err := write(uint8(len(opt.Value))); err != nil {
			return nil, err
		}

		if _, err := buf.Write(opt.Value); err != nil {
			return nil, err
		}
	}

	if err := buf.WriteByte(byte(OptionEndOption)); err != nil {
		return nil, err
	}
	
	return buf.Bytes(), nil
}

func DecodeOptions(data []byte) (Options, error) {
	var options Options
	i := 0
	for i < len(data) {
		code := OptionCode(data[i])
		i++
		if code == OptionEndOption {
			break
		}
		if code == OptionPad {
			continue
		}
		if i >= len(data) {
			return nil, fmt.Errorf("missing length for option %d", code)
		}
		length := int(data[i])
		i++
		if i+length > len(data) {
			return nil, fmt.Errorf("option %d length goes out of bounds", code)
		}
		value := data[i : i+length]
		i += length
		options = append(options, Option{Code: code, Value: value})
	}
	return options, nil
}

func (o *Options) Add(code OptionCode, value interface{}) {
	var val []byte
	switch v := value.(type) {
	case uint8:
		val = []byte{v}
	case uint16:
		val = make([]byte, 2)
		binary.BigEndian.PutUint16(val, v)
	case uint32:
		val = make([]byte, 4)
		binary.BigEndian.PutUint32(val, v)
	case int:
		val = make([]byte, 4)
		binary.BigEndian.PutUint32(val, uint32(v))
	case uint64:
		val = make([]byte, 8)
		binary.BigEndian.PutUint64(val, v)
	case []byte:
		val = v
	case string:
		val = []byte(v)
	case []string:
		val = []byte(strings.Join(v, " "))
	case net.IP:
		ip4 := v.To4()
		if ip4 != nil {
			val = ip4
		} else {
			val = v.To16()
		}
	case []net.IP:
		var buf bytes.Buffer
		for _, ip := range v {
			ip4 := ip.To4()
			if ip4 != nil {
				buf.Write(ip4)
			} else {
				buf.Write(ip.To16())
			}
		}
		val = buf.Bytes()
	default:
		log.Errorf("Unknown option type: %T with value: %v", value, value)
		return
	}
	*o = append(*o, Option{Code: code, Value: val})
}

func (o Options) GetMessageType() uint8 {
	for _, opt := range o {
		if opt.Code == OptionDHCPMessageType && len(opt.Value) == 1 {
			return opt.Value[0]
		}
	}
	return 0
}

func (o Options) GetRequestedIP() net.IP {
	for _, opt := range o {
		if opt.Code == OptionRequestedIPAddress && len(opt.Value) == 4 {
			return net.IP(opt.Value)
		}
	}
	return nil
}

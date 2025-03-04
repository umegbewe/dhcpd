package storage

import (
	"net"
	"time"
)

type Lease struct {
	IP       net.IP
	MAC      net.HardwareAddr
	ExpireAt time.Time
}

type LeaseStore interface {
	SaveLease(lease *Lease) error
	GetLeaseByMAC(mac net.HardwareAddr) (*Lease, error)
	GetLeaseByIP(ip net.IP) (*Lease, error)
	DeleteLease(ip net.IP) error
	ListLeases() ([]*Lease, error)
	Close() error
}

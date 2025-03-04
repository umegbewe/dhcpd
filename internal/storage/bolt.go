package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	// "github.com/umegbewe/dhcpd/pkg/dhcp"
	bolt "go.etcd.io/bbolt"
)

type BoltStore struct {
	db *bolt.DB
}

var (
	ipBucket  = []byte("leases_by_ip")
	macBucket = []byte("leases_by_mac")
)

func NewBoltStore(dbPath string) (*BoltStore, error) {
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(ipBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(macBucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return &BoltStore{db: db}, nil
}

func (s *BoltStore) SaveLease(lease *Lease) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		ipBkt := tx.Bucket(ipBucket)
		macBkt := tx.Bucket(macBucket)
		if ipBkt == nil || macBkt == nil {
			return errors.New("missing buckets in DB")
		}

		data, err := serializeLease(lease)
		if err != nil {
			return err
		}

		if e := ipBkt.Put(lease.IP.To4(), data); e != nil {
			return e
		}

		return macBkt.Put(lease.MAC, data)
	})
}

func (s *BoltStore) GetLeaseByMAC(mac net.HardwareAddr) (*Lease, error) {
	var lease *Lease
	err := s.db.View(func(tx *bolt.Tx) error {
		macBkt := tx.Bucket(macBucket)
		if macBkt == nil {
			return nil
		}

		data := macBkt.Get(mac)
		if data == nil {
			return nil
		}

		l, err := deserializeLease(data)
		if err != nil {
			return err
		}

		lease = l

		return nil
	})
	return lease, err
}

func (s *BoltStore) GetLeaseByIP(ip net.IP) (*Lease, error) {
	var lease *Lease
	err := s.db.View(func(tx *bolt.Tx) error {
		ipBkt := tx.Bucket(ipBucket)
		if ipBkt == nil {
			return nil
		}

		data := ipBkt.Get(ip.To4())
		if data == nil {
			return nil
		}

		l, err := deserializeLease(data)
		if err != nil {
			return err
		}

		lease = l
		return nil
	})
	return lease, err
}

func (s *BoltStore) DeleteLease(ip net.IP) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		ipBkt := tx.Bucket(ipBucket)
		macBkt := tx.Bucket(macBucket)

		if ipBkt == nil || macBkt == nil {
			return nil
		}
		data := ipBkt.Get(ip.To4())
		if data == nil {
			return nil
		}

		l, err := deserializeLease(data)
		if err != nil {
			return err
		}

		if err := ipBkt.Delete(ip.To4()); err != nil {
			return err
		}
		return macBkt.Delete(l.MAC)
	})
}

func (s *BoltStore) ListLeases() ([]*Lease, error) {
	var leases []*Lease
	err := s.db.View(func(tx *bolt.Tx) error {
		ipBkt := tx.Bucket(ipBucket)
		if ipBkt == nil {
			return nil
		}
		c := ipBkt.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			l, err := deserializeLease(v)
			if err != nil {
				continue
			}
			leases = append(leases, l)
		}
		return nil
	})
	return leases, err
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}

func serializeLease(l *Lease) ([]byte, error) {
	ip := l.IP.To4()
	if ip == nil {
		return nil, fmt.Errorf("invalid IP: %v", l.IP)
	}

	out := make([]byte, 18)
	copy(out[0:4], ip)
	if len(l.MAC) < 6 {
		return nil, fmt.Errorf("MAC too short: %s", l.MAC)
	}

	copy(out[4:10], l.MAC[:6])
	expireSec := l.ExpireAt.Unix()
	binary.BigEndian.PutUint64(out[10:18], uint64(expireSec))

	return out, nil
}

func deserializeLease(data []byte) (*Lease, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("invalid data length for lease, want >=18, got %d", len(data))
	}

	ip := net.IPv4(data[0], data[1], data[2], data[3])
	mac := net.HardwareAddr(data[4:10])
	expireSec := int64(binary.BigEndian.Uint64(data[10:18]))

	return &Lease{
		IP:       ip,
		MAC:      mac,
		ExpireAt: time.Unix(expireSec, 0),
	}, nil
}

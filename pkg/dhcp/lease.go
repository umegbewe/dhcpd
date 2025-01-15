package dhcp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/umegbewe/dhcpd/internal/bitmap"
	"github.com/umegbewe/dhcpd/internal/metrics"
	bolt "go.etcd.io/bbolt"
)

type Lease struct {
	IP       net.IP
	MAC      net.HardwareAddr
	ExpireAt time.Time
}

type Pool struct {
	start       net.IP
	end         net.IP
	leaseTime   time.Duration
	db          *bolt.DB
	mutex       sync.RWMutex
	used        *bitmap.Bitmap
	nextIdx     int
	totalLeases int
}

var (
	ipBucket  = []byte("leases_by_ip")
	macBucket = []byte("leases_by_mac")
)

func LeasePool(start, end string, leaseTime int, dbPath string) (*Pool, error) {
	startIP := net.ParseIP(start).To4()
	endIP := net.ParseIP(end).To4()

	if startIP == nil || endIP == nil {
		return nil, errors.New("invalid ip range")
	}

	startIPInt := ipToUint32(startIP)
	endIPInt := ipToUint32(endIP)
	if endIPInt < startIPInt {
		return nil, errors.New("end IP must be >= start IP")
	}

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
		return nil, err
	}

	total := int(endIPInt - startIPInt + 1)
	p := &Pool{
		start:       startIP,
		end:         endIP,
		leaseTime:   time.Duration(leaseTime) * time.Second,
		db:          db,
		used:        bitmap.BitMap(total),
		nextIdx:     0,
		totalLeases: total,
	}

	now := time.Now()
	err = db.View(func(tx *bolt.Tx) error {
		ipBkt := tx.Bucket(ipBucket)
		if ipBkt == nil {
			return nil
		}
		c := ipBkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			l, de := deserialize(v)
			if de != nil {
				continue // skip corrupt
			}
			// if the lease has not expired, remove from freeIPs
			if now.Before(l.ExpireAt) {
				offset := ipToUint32(l.IP) - startIPInt
				if int(offset) < total {
					p.used.Set(int(offset))
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Pool) Allocate(iface *net.Interface, mac net.HardwareAddr) (net.IP, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// first, check if there's a record for this MAC do a direct read from DB so we can see if it is expired or not
	var storedLease *Lease
	_ = p.db.View(func(tx *bolt.Tx) error {
		macBkt := tx.Bucket(macBucket)
		if macBkt == nil {
			return nil
		}
		val := macBkt.Get(mac)
		if val == nil {
			return nil
		}
		l, decErr := deserialize(val)
		if decErr == nil {
			storedLease = l
		}
		return nil
	})

	// if storedLease == nil, there's no record at all (MAC not in DB) if it is non-nil, it might be expired or unexpired so let's see
	now := time.Now()
	if storedLease != nil {
		if now.Before(storedLease.ExpireAt) {
			//the existing lease is still valid => just renew it
			storedLease.ExpireAt = now.Add(p.leaseTime)
			if err := p.saveLease(storedLease); err != nil {
				return nil, err
			}

			metrics.LeaseRenewals.Inc()
			return storedLease.IP, nil
		}

		expiredIP := storedLease.IP

		// if we get here, the lease is expired => free that IP
		_ = p.db.Update(func(tx *bolt.Tx) error {
			ipBkt := tx.Bucket(ipBucket)
			macBkt := tx.Bucket(macBucket)
			if ipBkt != nil && macBkt != nil {
				_ = ipBkt.Delete(expiredIP.To4())
				_ = macBkt.Delete(storedLease.MAC)
			}
			return nil
		})

		// clear bit
		startUint := ipToUint32(p.start)
		offset := ipToUint32(expiredIP) - startUint
		if int(offset) < p.totalLeases {
			p.used.Clear(int(offset))
		}

	}

	// need a brand new IP => find the next free offset in the bitmap
	currentSearchIdx := p.nextIdx
	freeOffset := p.used.FindNextClearBit(currentSearchIdx)
	if freeOffset == -1 {
		return nil, errors.New("no available lease")
	}

	ip := offsetToIP(p.start, freeOffset)

	// loopback interfaces should never do arp resolution
	if iface.Name != "lo0" && iface.Name != "lo" {
		inUse, err := ARPCheck(iface, ip, 2*time.Second)
		if err != nil {
			// ARP check failed, bail out.
			return nil, fmt.Errorf("arp check error: %v", err)
		}
		if inUse {
			metrics.ArpCheckFailures.Inc()
			// mark IP as used anyway, so we don’t pick it again.
			p.used.Set(freeOffset)
			// move forward and try the next one next time
			p.nextIdx = (freeOffset + 1) % p.totalLeases
			return nil, fmt.Errorf("[WARN] IP %s in use (ARP reply). No lease allocated", ip)
		}
	}

	p.used.Set(freeOffset)
	p.nextIdx = (freeOffset + 1) % p.totalLeases

	newLease := &Lease{
		IP:       ip,
		MAC:      mac,
		ExpireAt: time.Now().Add(p.leaseTime),
	}

	if err := p.saveLease(newLease); err != nil {
		p.used.Clear(freeOffset)
		return nil, err
	}

	activeLeaseCount := p.countActiveLeases()
	metrics.AvailableLeases.Set(float64(p.totalLeases - activeLeaseCount))
	metrics.ActiveLeases.Set(float64(activeLeaseCount))

	return ip, nil
}

func (p *Pool) Release(ip net.IP) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	err := p.db.Update(func(tx *bolt.Tx) error {
		ipBkt := tx.Bucket(ipBucket)
		macBkt := tx.Bucket(macBucket)

		if ipBkt == nil || macBkt == nil {
			return nil
		}

		val := ipBkt.Get(ip.To4())
		if val == nil {
			// IP not found, nothing to do
			return nil
		}
		l, de := deserialize(val)
		if de != nil {
			return de
		}

		// delete from both IP and MAC buckets
		if e := ipBkt.Delete(ip.To4()); e != nil {
			return e
		}
		return macBkt.Delete(l.MAC)
	})
	if err != nil {
		return err
	}

	// clear bit
	startUint := ipToUint32(p.start)
	offset := int(ipToUint32(ip) - startUint)
	if offset >= 0 && offset < p.totalLeases {
		p.used.Clear(offset)
	}

	activeLeaseCount := p.countActiveLeases()
	metrics.AvailableLeases.Set(float64(p.totalLeases - activeLeaseCount))
	metrics.ActiveLeases.Set(float64(activeLeaseCount))

	return nil
}

func (p *Pool) GetLeaseByMAC(mac net.HardwareAddr) *Lease {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	var lease *Lease
	_ = p.db.View(func(tx *bolt.Tx) error {
		macBkt := tx.Bucket(macBucket)
		val := macBkt.Get(mac)
		if val != nil {
			l, err := deserialize(val)
			if err == nil && time.Now().Before(l.ExpireAt) {
				lease = l
			}
		}
		return nil
	})
	return lease
}

func (p *Pool) ListLeases() []*Lease {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	var leases []*Lease
	_ = p.db.View(func(tx *bolt.Tx) error {
		ipBkt := tx.Bucket(ipBucket)
		if ipBkt == nil {
			return nil
		}
		c := ipBkt.Cursor()
		now := time.Now()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			l, err := deserialize(v)
			if err != nil {
				continue
			}
			if now.Before(l.ExpireAt) {
				leases = append(leases, l)
			}
		}
		return nil
	})
	return leases
}

func (p *Pool) saveLease(l *Lease) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		ipBkt := tx.Bucket(ipBucket)
		macBkt := tx.Bucket(macBucket)
		if ipBkt == nil || macBkt == nil {
			return errors.New("missing buckets in DB")
		}

		data, err := l.serialize()
		if err != nil {
			return err
		}

		if e := ipBkt.Put(l.IP.To4(), data); e != nil {
			return e
		}

		return macBkt.Put(l.MAC, data)
	})
}

func (l *Lease) serialize() ([]byte, error) {
	ip := l.IP.To4()
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

func deserialize(data []byte) (*Lease, error) {
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

func (p *Pool) Close() error {
	return p.db.Close()
}

func (p *Pool) StartCleanup(interval time.Duration) {
	log.Println("Starting cleanup of expired leases")
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			if err := p.CleanupExpiredLeases(); err != nil {
				log.Errorf("Lease cleanup: %v", err)
			}
		}
	}()
}

func (p *Pool) CleanupExpiredLeases() error {
	now := time.Now()
	var freedOffsets []int

	startUint := ipToUint32(p.start)

	err := p.db.Update(func(tx *bolt.Tx) error {
		ipBkt := tx.Bucket(ipBucket)
		macBkt := tx.Bucket(macBucket)
		if ipBkt == nil || macBkt == nil {
			return nil
		}

		c := ipBkt.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			l, de := deserialize(v)

			if de != nil {
				continue	// skip corrupt
			}
			if now.After(l.ExpireAt) {
				if err := c.Delete(); err != nil {
					return err
				}
				if err := macBkt.Delete(l.MAC); err != nil {
					return err
				}

				 // track offset for clearing in bitmap
				offset := int(ipToUint32(l.IP) - startUint)
				if offset >= 0 && offset < p.totalLeases {
					freedOffsets = append(freedOffsets, offset)
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// clear bit
	if len(freedOffsets) > 0 {
		p.mutex.Lock()
		defer p.mutex.Unlock()

		for _, off := range freedOffsets {
			p.used.Clear(off)
		}

		metrics.LeaseExpirations.Add(float64(len(freedOffsets)))

		activeLeaseCount := p.countActiveLeases()
		metrics.AvailableLeases.Set(float64(p.totalLeases - activeLeaseCount))
		metrics.ActiveLeases.Set(float64(activeLeaseCount))
	}

	return nil
}

func (p *Pool) countActiveLeases() int {
	used := 0
	for i := 0; i < p.totalLeases; i++ {
		if p.used.IsSet(i) {
			used++
		}
	}
	return used
}

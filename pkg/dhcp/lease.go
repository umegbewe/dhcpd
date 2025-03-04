package dhcp

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/umegbewe/dhcpd/internal/bitmap"
	"github.com/umegbewe/dhcpd/internal/metrics"
	"github.com/umegbewe/dhcpd/internal/storage"
)

type Pool struct {
	start       net.IP
	end         net.IP
	leaseTime   time.Duration
	store       storage.LeaseStore
	mutex       sync.RWMutex
	used        *bitmap.Bitmap
	nextIdx     int
	totalLeases int
}

func LeasePool(start, end string, leaseTime int, store storage.LeaseStore) (*Pool, error) {
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

	total := int(endIPInt - startIPInt + 1)
	p := &Pool{
		start:       startIP,
		end:         endIP,
		leaseTime:   time.Duration(leaseTime) * time.Second,
		store:       store,
		used:        bitmap.BitMap(total),
		nextIdx:     0,
		totalLeases: total,
	}

	leases, err := store.ListLeases()
	if err != nil {
		return nil, fmt.Errorf("failed to load leases: %v", err)
	}

	now := time.Now()
	for _, l := range leases {
		if now.Before(l.ExpireAt) {
			offset := ipToUint32(l.IP) - startIPInt
			if int(offset) < total {
				p.used.Set(int(offset))
			}
		}
	}
	return p, nil
}

func (p *Pool) Allocate(iface *net.Interface, mac net.HardwareAddr, arp bool) (net.IP, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	storedLease, err := p.store.GetLeaseByMAC(mac)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if storedLease != nil {
		if now.Before(storedLease.ExpireAt) {
			// existing lease is valid => renew
			storedLease.ExpireAt = now.Add(p.leaseTime)
			if err := p.store.SaveLease(storedLease); err != nil {
				return nil, err
			}

			metrics.LeaseRenewals.Inc()
			return storedLease.IP, nil
		}

		if err := p.store.DeleteLease(storedLease.IP); err != nil {
			log.Warnf("Failed to delete expired lease for IP %s: %v", storedLease.IP, err)
		}

		offset := ipToUint32(storedLease.IP) - ipToUint32(p.start)
		if int(offset) < p.totalLeases {
			p.used.Clear(int(offset))
		}
	}

	// need a brand new IP => find the next free offset in the bitmap
	freeOffset := p.used.FindNextClearBit(p.nextIdx)
	if freeOffset == -1 {
		return nil, errors.New("no available lease")
	}
	ip := offsetToIP(p.start, freeOffset)

	// loopback interfaces should never do arp resolution
	if arp && iface.Name != "lo0" && iface.Name != "lo" {
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

	newLease := &storage.Lease{
		IP:       ip,
		MAC:      mac,
		ExpireAt: time.Now().Add(p.leaseTime),
	}

	if err := p.store.SaveLease(newLease); err != nil {
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

	if err := p.store.DeleteLease(ip); err != nil {
		return err
	}

	offset := int(ipToUint32(ip)) - int(ipToUint32(p.start))
	if offset >= 0 && offset < p.totalLeases {
		p.used.Clear(offset)
	}

	activeLeaseCount := p.countActiveLeases()
	metrics.AvailableLeases.Set(float64(p.totalLeases - activeLeaseCount))
	metrics.ActiveLeases.Set(float64(activeLeaseCount))

	return nil
}

func (p *Pool) GetLeaseByMAC(mac net.HardwareAddr) *storage.Lease {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	lease, err := p.store.GetLeaseByMAC(mac)
	if err != nil || lease == nil || time.Now().After(lease.ExpireAt) {
		return nil
	}

	return lease
}

func (p *Pool) ListLeases() []*storage.Lease {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	leases, err := p.store.ListLeases()
	if err != nil {
		log.Errorf("Failed to list leases: %v", err)
		return nil
	}

	now := time.Now()
	var activeLeases []*storage.Lease
	for _, l := range leases {
		if now.Before(l.ExpireAt) {
			activeLeases = append(activeLeases, l)
		}
	}
	return activeLeases
}

func (p *Pool) Close() error {
	return p.store.Close()
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
	leases, err := p.store.ListLeases()
	if err != nil {
		return err
	}

	now := time.Now()
	var freedOffsets []int
	startUint := ipToUint32(p.start)
	for _, l := range leases {
		if now.After(l.ExpireAt) {
			if err := p.store.DeleteLease(l.IP); err != nil {
				log.Warnf("Failed to delete expired lease %s: %v", l.IP, err)
				continue
			}
			offset := int(ipToUint32(l.IP) - startUint)
			if offset >= 0 && offset < p.totalLeases {
				freedOffsets = append(freedOffsets, offset)
			}
		}
	}

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

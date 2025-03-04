package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/go-redis/redis/v8"
)

type RedisStore struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisStore(addr, password string, db int) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	ctx := context.Background()
	_, err := client.Ping(ctx).Result()
	if err != nil {
		return nil, err
	}

	return &RedisStore{client: client, ctx: ctx}, nil
}

func (s *RedisStore) SaveLease(lease *Lease) error {
	data, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	ipKey := leaseKeyByIP(lease.IP)
	macKey := leaseKeyByMAC(lease.MAC)

	pipe := s.client.TxPipeline()
	pipe.Set(s.ctx, ipKey, data, 0)
	pipe.Set(s.ctx, macKey, data, 0)
	_, err = pipe.Exec(s.ctx)
	return err
}

func (s *RedisStore) GetLeaseByMAC(mac net.HardwareAddr) (*Lease, error) {
	macKey := leaseKeyByMAC(mac)
	data, err := s.client.Get(s.ctx, macKey).Bytes()

	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var lease Lease
	if err := json.Unmarshal(data, &lease); err != nil {
		return nil, err
	}
	return &lease, nil
}

func (s *RedisStore) GetLeaseByIP(ip net.IP) (*Lease, error) {
	ipKey := leaseKeyByIP(ip)
	data, err := s.client.Get(s.ctx, ipKey).Bytes()

	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var lease Lease
	if err := json.Unmarshal(data, &lease); err != nil {
		return nil, err
	}
	return &lease, nil
}

func (s *RedisStore) DeleteLease(ip net.IP) error {
	ipKey := leaseKeyByIP(ip)
	data, err := s.client.Get(s.ctx, ipKey).Bytes()
	if err == redis.Nil {
		return nil
	} else if err != nil {
		return err
	}
	var lease Lease
	if err := json.Unmarshal(data, &lease); err != nil {
		return err
	}
	macKey := leaseKeyByMAC(lease.MAC)
	pipe := s.client.TxPipeline()
	pipe.Del(s.ctx, ipKey)
	pipe.Del(s.ctx, macKey)
	_, err = pipe.Exec(s.ctx)
	return err
}

func (s *RedisStore) ListLeases() ([]*Lease, error) {
	var leases []*Lease
	var cursor uint64

	for {
		keys, nextCursor, err := s.client.Scan(s.ctx, cursor, "dhcp:lease:ip:*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			data, err := s.client.Get(s.ctx, key).Bytes()
			if err != nil {
				continue
			}
			var lease Lease
			if json.Unmarshal(data, &lease) == nil {
				leases = append(leases, &lease)
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return leases, nil
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func leaseKeyByIP(ip net.IP) string {
	return fmt.Sprintf("dhcp:lease:ip:%s", ip.String())
}

func leaseKeyByMAC(mac net.HardwareAddr) string {
	return fmt.Sprintf("dhcp:lease:mac:%s", mac.String())
}

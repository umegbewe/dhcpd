package storage

import (
	"database/sql"
	"net"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type SqliteStore struct {
	db *sql.DB
}

func NewSqliteStore(path string) (*SqliteStore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	err = createTable(db)
	if err != nil {
		return nil, err
	}

	return &SqliteStore{db}, nil
}

func createTable(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS leases (
    ipkey TEXT NOT NULL UNIQUE PRIMARY KEY,
    mackey TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err := db.Exec(query)
	return err
}

func (s *SqliteStore) SaveLease(lease *Lease) error {
	query := `INSERT INTO leases (ipkey, mackey, expires_at, created_at)
		VALUES (?, ?, ?, ?);
	`

	_, err := s.db.Exec(query, lease.IP.String(), lease.MAC.String(), lease.ExpireAt, time.Now())
	return err
}

func (s *SqliteStore) GetLeaseByIP(ip net.IP) (*Lease, error) {
	var lease Lease
	var mackey string

	query := `SELECT mackey, expires_at FROM leases WHERE ipkey = ?;`
	row := s.db.QueryRow(query, ip.String())
	err := row.Scan(
		&mackey,
		&lease.ExpireAt,
	)
	if err != nil {
		return nil, err
	}

	lease.IP = ip
	lease.MAC, _ = net.ParseMAC(mackey)

	return &lease, nil
}

func (s *SqliteStore) GetLeaseByMAC(mac net.HardwareAddr) (*Lease, error) {
	var lease Lease
	var ipkey string

	query := `SELECT ipkey, expires_at FROM leases WHERE mackey = ?;`
	row := s.db.QueryRow(query, mac.String())
	err := row.Scan(
		&ipkey,
		&lease.ExpireAt,
	)
	if err != nil {
		return nil, err
	}

	lease.IP = net.ParseIP(ipkey)
	lease.MAC = mac

	return &lease, nil
}

func (s *SqliteStore) DeleteLease(ip net.IP) error {
	query := `DELETE FROM leases WHERE ipkey = ?;`
	_, err := s.db.Exec(query, ip.String())
	return err
}

func (s *SqliteStore) ListLeases() ([]*Lease, error) {
	var leases []*Lease

	query := `SELECT ipkey, mackey, expires_at FROM leases;`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var lease Lease
		var ipkey string
		var mackey string
		err := rows.Scan(
			&ipkey,
			&mackey,
			&lease.ExpireAt,
		)
		if err != nil {
			return nil, err
		}
		lease.IP = net.ParseIP(ipkey)
		lease.MAC, _ = net.ParseMAC(mackey)
		leases = append(leases, &lease)
	}

	return leases, nil
}

func (s *SqliteStore) Close() error {
	return s.db.Close()
}

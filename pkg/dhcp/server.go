package dhcp

import (
	"errors"
	"fmt"
	"net"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/umegbewe/dhcpd/internal/config"
	"github.com/umegbewe/dhcpd/internal/logging"
	"github.com/umegbewe/dhcpd/internal/metrics"
	"github.com/umegbewe/dhcpd/internal/storage"
)

type Server struct {
	Config         *config.Config
	LeasePool      *Pool
	Connection     *net.UDPConn
	Interface      *net.Interface
	metricsEnabled bool
}

func InitServer(cfg *config.Config) (*Server, error) {
	err := logging.SetupLogging(cfg.Logging.Level)
	if err != nil {
		log.Fatalf("Failed to setup logging: %v", err)
	}

	var store storage.LeaseStore
	switch cfg.Database.Type {
	case "bolt":
		if cfg.Database.Bolt.Path == "" {
			return nil, fmt.Errorf("bolt database path is required")
		}
		store, err = storage.NewBoltStore(cfg.Database.Bolt.Path)
	case "sqlite":
		if cfg.Database.Sqlite.Path == "" {
			return nil, fmt.Errorf("sqlite database path is required")
		}
		store, err = storage.NewSqliteStore(cfg.Database.Sqlite.Path)
	case "redis":
		store, err = storage.NewRedisStore(cfg.Database.Redis.Addr, cfg.Database.Redis.Password, cfg.Database.Redis.DB)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Database.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize lease store: %v", err)
	}

	pool, err := LeasePool(cfg.Server.IPStart, cfg.Server.IPEnd, cfg.Server.LeaseTime, store)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create lease pool: %v", err)
	}

	iface, err := net.InterfaceByName(cfg.Server.Interface)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("[ERROR] Could not find interface %s: %v", cfg.Server.Interface, err)
	}

	if cfg.Server.CleanupExpiredInterval > 0 {
		log.Infof("[INIT] Set to cleanup expired leases every %d seconds", cfg.Server.CleanupExpiredInterval)
		pool.StartCleanup(time.Duration(cfg.Server.CleanupExpiredInterval) * time.Second)
	}

	server := &Server{
		Config:         cfg,
		LeasePool:      pool,
		Interface:      iface,
		metricsEnabled: cfg.Metrics.Enabled,
	}

	if server.metricsEnabled {
		if err := metrics.StartMetricsServer(cfg.Metrics.ListenAddress); err != nil {
			log.Warnf("Could not start metrics server: %v", err)
		} else {
			log.Infof("[INIT] Metrics server listening on %s", cfg.Metrics.ListenAddress)
		}
	}

	return server, nil
}

func (s *Server) Start() error {
	defer s.LeasePool.Close()

	addrs, err := s.Interface.Addrs()
	if err != nil {
		return fmt.Errorf("[ERROR] Could not get addresses for interface %s: %v", s.Config.Server.Interface, err)
	}

	var ip net.IP
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			ip = ipnet.IP
			break
		}
	}

	if ip == nil {
		return fmt.Errorf("no IPv4 address found for interface %s", s.Config.Server.Interface)
	}

	addr := &net.UDPAddr{IP: net.IPv4zero, Port: s.Config.Server.Port}
	s.Connection, err = net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP socket: %v", err)
	}
	defer s.Connection.Close()

	log.Infof("[INIT] DHCP server listening on %s (interface: %s, IP: %s)", addr, s.Config.Server.Interface, ip)

	for {
		buffer := make([]byte, 1500)
		n, remoteAddr, err := s.Connection.ReadFromUDP(buffer)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				if opErr.Err.Error() == "use of closed network connection" || errors.Is(opErr.Err, net.ErrClosed) {
					log.Infof("[INFO] Server shutting down")
					break
				}
			}
			log.Errorf("[ERROR] Error reading from UDP: %v", err)
			continue
		}
		go s.HandleMessage(buffer[:n], remoteAddr)
	}
	return nil
}

func (s *Server) HandleMessage(data []byte, remoteAddr *net.UDPAddr) {
	start := time.Now()

	message, err := DecodeMessage(data)
	if err != nil {
		log.Errorf("Error decoding message from %v: %v", remoteAddr, err)
		return
	}

	msgType := message.Options.GetMessageType()

	if s.metricsEnabled {
		metrics.MessageTypeCount.WithLabelValues(messageTypeToString(msgType)).Inc()
	}

	switch msgType {
	case MessageTypeDiscover:
		s.HandleDiscover(message, remoteAddr)
	case MessageTypeRequest:
		s.HandleRequest(message, remoteAddr)
	case MessageTypeRelease:
		s.HandleRelease(message)
	case MessageTypeDecline:
		s.HandleDecline(message)
	case MessageTypeInform:
		s.HandleInform(message, remoteAddr)
	default:
		log.Infof("[INFO] Ignoring unsupported DHCP message type=%d", msgType)
	}

	if s.metricsEnabled {
		duration := time.Since(start).Seconds()
		metrics.MessageLatency.WithLabelValues(messageTypeToString(msgType)).Observe(duration)
	}
}

func (s *Server) HandleDiscover(message *Message, remoteAddr *net.UDPAddr) {
	log.Infof("[DHCPDISCOVER] from MAC=%s;", message.CHAddr)

	ip, err := s.LeasePool.Allocate(s.Interface, message.CHAddr, s.Config.Server.ARPCheck)
	if err != nil {
		log.Errorf("Error allocating lease: %v", err)
		return
	}

	reply := NewMessage()
	reply.XID = message.XID
	reply.Flags = message.Flags
	reply.CHAddr = make([]byte, 16)
	copy(reply.CHAddr, message.CHAddr)
	reply.YIAddr = ip
	reply.Options.Add(OptionDHCPMessageType, uint8(MessageTypeOffer))
	reply.Options.Add(OptionSubnetMask, net.ParseIP(s.Config.Server.SubnetMask).To4())
	reply.Options.Add(OptionRouterAddress, net.ParseIP(s.Config.Server.Gateway).To4())
	for _, dns := range s.Config.Server.DNSServers {
		reply.Options.Add(OptionDNSServers, net.ParseIP(dns).To4())
	}

	if s.Config.Server.TFTPServerName != "" {
		reply.Options.Add(OptionTFTPServerName, s.Config.Server.TFTPServerName)
	}
	if s.Config.Server.BootFileName != "" {
		reply.Options.Add(OptionBootfileName, s.Config.Server.BootFileName)
	}

	reply.Options.Add(OptionIPAddressLeaseTime, uint32(s.Config.Server.LeaseTime))
	reply.Options.Add(OptionServerIdentifier, net.ParseIP(s.Config.Server.ServerIP).To4())

	s.SendReply(reply, message)
}

func (s *Server) HandleRequest(message *Message, remoteAddr *net.UDPAddr) {
	log.Infof("[DHCPREQUEST] from MAC=%s", message.CHAddr)

	requestedIP := message.Options.GetRequestedIP()
	if requestedIP != nil {
		log.Infof("Client specifically requested IP=%s", requestedIP)
	}

	leases := s.LeasePool.GetLeaseByMAC(message.CHAddr)
	if leases == nil {
		// client is new or changed MACs. We can attempt new allocation
		ip, err := s.LeasePool.Allocate(s.Interface, message.CHAddr, s.Config.Server.ARPCheck)
		if err != nil {
			s.SendNAK(message, "No available IP")
			return
		}
		leases = s.LeasePool.GetLeaseByMAC(message.CHAddr)
		log.Infof("Allocated new IP=%s to MAC=%s", ip, message.CHAddr)
		if leases == nil {
			log.Warnf("[NAK] Lease not found immediately after allocation!")
			s.SendNAK(message, "Lease retrieval error")
			return
		}

	}

	if requestedIP != nil && !leases.IP.Equal(requestedIP) {
		log.Warnf("Client requested IP=%s but lease DB says IP=%s", requestedIP, leases.IP)
		s.SendNAK(message, "Requested IP mismatch with lease")
	}

	reply := NewMessage()
	reply.XID = message.XID
	reply.YIAddr = leases.IP
	reply.CHAddr = make([]byte, 16)
	copy(reply.CHAddr, message.CHAddr)
	reply.Options.Add(OptionDHCPMessageType, uint8(MessageTypeAck))
	reply.Options.Add(OptionSubnetMask, net.ParseIP(s.Config.Server.SubnetMask).To4())
	reply.Options.Add(OptionRouterAddress, net.ParseIP(s.Config.Server.Gateway).To4())
	for _, dns := range s.Config.Server.DNSServers {
		reply.Options.Add(OptionDNSServers, net.ParseIP(dns).To4())
	}
	if s.Config.Server.TFTPServerName != "" {
		reply.Options.Add(OptionTFTPServerName, s.Config.Server.TFTPServerName)
	}
	if s.Config.Server.BootFileName != "" {
		reply.Options.Add(OptionBootfileName, s.Config.Server.BootFileName)
	}
	reply.Options.Add(OptionIPAddressLeaseTime, uint32(s.Config.Server.LeaseTime))
	reply.Options.Add(OptionServerIdentifier, net.ParseIP(s.Config.Server.ServerIP).To4())

	s.SendReply(reply, message)
}

func (s *Server) HandleRelease(message *Message) {
	log.Infof("Received DHCP Release from MAC %s, IP %s", message.CHAddr, message.CIAddr)
	err := s.LeasePool.Release(message.CIAddr)
	if err != nil {
		log.Errorf("Error releasing IP %s: %v", message.CIAddr, err)
	}
}

func (s *Server) HandleDecline(message *Message) {
	log.Infof("Received DHCP Decline from MAC %s, IP %s", message.CHAddr, message.CIAddr)
	_ = s.LeasePool.Release(message.CIAddr)
}

func (s *Server) HandleInform(message *Message, remoteAddr *net.UDPAddr) {
	log.Infof("Received DHCP Inform from MAC %s", message.CHAddr)
	reply := NewMessage()
	reply.XID = message.XID
	reply.CIAddr = message.CIAddr

	reply.Options.Add(OptionDHCPMessageType, uint8(MessageTypeAck))
	reply.Options.Add(OptionSubnetMask, s.Config.Server.SubnetMask)
	reply.Options.Add(OptionRouterAddress, s.Config.Server.Gateway)

	s.SendReply(reply, message)
}

func (s *Server) SendNAK(message *Message, reason string) {
	log.Infof("Sending DHCP NAK to MAC %s: %s", message.CHAddr, reason)

	if s.metricsEnabled {
		metrics.DHCPNAKs.Inc()
	}

	reply := NewMessage()
	reply.XID = message.XID
	reply.Options.Add(OptionDHCPMessageType, uint8(MessageTypeNak))

	s.SendReply(reply, message)
}

func (s *Server) SendReply(reply *Message, req *Message) {
	d := s.determineDestAddr(req)

	reply.XID = req.XID
	reply.Flags = req.Flags
	copy(reply.CHAddr, req.CHAddr)

	raw, err := reply.Encode()
	if err != nil {
		log.Errorf("Encoding reply failed %v", err)
	}

	if _, err := s.Connection.WriteToUDP(raw, d); err != nil {
		log.Warnf("Could not send reply to %v: %v", d, err)
	}
}

func (s *Server) determineDestAddr(req *Message) *net.UDPAddr {
	if !req.GIAddr.Equal(net.IPv4zero) {
		return &net.UDPAddr{IP: req.GIAddr, Port: 67}
	}
	if (req.Flags & 0x8000) != 0 {
		return &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	}
	if !req.CIAddr.Equal(net.IPv4zero) {
		return &net.UDPAddr{IP: req.CIAddr, Port: 68}
	}
	return &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
}

func (s *Server) Shutdown() {
	if s.Connection != nil {
		s.Connection.Close()
	}
	if s.LeasePool != nil {
		s.LeasePool.Close()
	}
}

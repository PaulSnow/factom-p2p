package p2p

import (
	"encoding/gob"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

var peerLogger = packageLogger.WithField("subpack", "peer")

// Peer is an active connection to an endpoint in the network
type Peer struct {
	net  *Network
	conn net.Conn
	//handshake Handshake

	// current state
	IsIncoming bool

	stopper      sync.Once
	stop         chan bool
	stopDelivery chan bool

	lastPeerRequest time.Time
	peerShareAsk    bool
	lastPeerSend    time.Time
	send            ParcelChannel
	error           chan error
	disconnect      chan *Peer

	encoder *gob.Encoder
	decoder *gob.Decoder

	connectionAttempt      time.Time
	connectionAttemptCount uint

	Seed bool

	IP     IP
	NodeID uint64 // a nonce to distinguish multiple nodes behind one IP address
	Hash   string // This is more of a connection ID than hash right now.

	// Metrics
	metricsMtx      sync.RWMutex
	Connected       time.Time
	QualityScore    int32     // 0 is neutral quality, negative is a bad peer.
	LastReceive     time.Time // Keep track of how long ago we talked to the peer.
	LastSend        time.Time // Keep track of how long ago we talked to the peer.
	ParcelsSent     uint64
	ParcelsReceived uint64
	BytesSent       uint64
	BytesReceived   uint64

	// logging
	logger *log.Entry
}

type PeerError struct {
	Peer *Peer
	err  error
}

func NewPeer(net *Network, disconnect chan *Peer) *Peer {
	p := &Peer{}
	p.net = net
	p.disconnect = disconnect

	p.logger = peerLogger.WithFields(log.Fields{
		"node":    net.conf.NodeName,
		"hash":    p.Hash,
		"address": p.IP.Address,
		"Port":    p.IP.Port,
		//"version": p.handshake.Version,
	})
	p.stop = make(chan bool, 1)
	p.stopDelivery = make(chan bool, 1)

	p.logger.Debugf("Creating blank peer")
	return p
}

func (p *Peer) StartWithHandshakeV10(ip IP, con net.Conn, incoming bool) bool {
	tmplogger := p.logger.WithField("addr", ip.Address)
	timeout := time.Now().Add(p.net.conf.HandshakeTimeout)
	handshake := Handshake{
		NodeID:  p.net.conf.NodeID,
		Port:    p.net.conf.ListenPort,
		Version: p.net.conf.ProtocolVersion,
		Network: p.net.conf.Network}

	p.decoder = gob.NewDecoder(con)
	p.encoder = gob.NewEncoder(con)
	con.SetWriteDeadline(timeout)
	con.SetReadDeadline(timeout)
	err := p.encoder.Encode(handshake)

	if err != nil {
		tmplogger.WithError(err).Debugf("Failed to send handshake to incoming connection")
		con.Close()
		return false
	}

	err = p.decoder.Decode(&handshake)
	if err != nil {
		tmplogger.WithError(err).Debugf("Failed to read handshake from incoming connection")
		con.Close()
		return false
	}

	err = handshake.Verify(p.net.conf.NodeID, p.net.conf.ProtocolVersionMinimum, p.net.conf.Network)
	if err != nil {
		tmplogger.WithError(err).Debug("Handshake failed")
		con.Close()
		return false
	}

	ip.Port = handshake.Port
	//p.handshake = handshake
	p.IP = ip
	p.NodeID = handshake.NodeID
	p.Hash = fmt.Sprintf("%s:%s %016x", ip.Address, ip.Port, p.NodeID)
	p.send = NewParcelChannel(p.net.conf.ChannelCapacity)
	p.error = make(chan error, 10)
	p.IsIncoming = incoming
	p.conn = con
	p.Connected = time.Now()

	go p.sendLoop()
	go p.readLoop()

	return true
}

func (p *Peer) StartWithHandshake(ip IP, con net.Conn, incoming bool) bool {
	tmplogger := p.logger.WithField("addr", ip.Address)
	timeout := time.Now().Add(p.net.conf.HandshakeTimeout)
	request := NewParcel(TypePeerRequest, []byte("Peer Request"))
	request.Header.Version = p.net.conf.ProtocolVersion
	request.Header.Network = p.net.conf.Network
	request.Header.PeerPort = p.net.conf.ListenPort

	p.decoder = gob.NewDecoder(con)
	p.encoder = gob.NewEncoder(con)
	con.SetWriteDeadline(timeout)
	con.SetReadDeadline(timeout)
	err := p.encoder.Encode(request)

	if err != nil {
		tmplogger.WithError(err).Debugf("Failed to send handshake to incoming connection")
		con.Close()
		return false
	}

	err = p.decoder.Decode(&request)
	if err != nil {
		tmplogger.WithError(err).Debugf("Failed to read handshake from incoming connection")
		con.Close()
		return false
	}

	failfunc := func(err error) bool {
		tmplogger.WithError(err).Debug("Handshake failed")
		con.Close()
		return false
	}

	h := request.Header
	if h.NodeID == p.net.conf.NodeID {
		return failfunc(fmt.Errorf("connected to ourselves"))
	}

	if h.Version < p.net.conf.ProtocolVersionMinimum {
		return failfunc(fmt.Errorf("version %d is below the minimum", h.Version))
	}

	if h.Network != p.net.conf.Network {
		return failfunc(fmt.Errorf("wrong network id %x", h.Network))
	}

	port, err := strconv.Atoi(h.PeerPort)
	if err != nil {
		return failfunc(fmt.Errorf("unable to parse port %s: %v", h.PeerPort, err))
	}

	if port < 1 || port > 65535 {
		return failfunc(fmt.Errorf("given port out of range: %d", port))
	}

	ip.Port = h.PeerPort
	//p.handshake = handshake
	p.IP = ip
	p.NodeID = h.NodeID
	p.Hash = fmt.Sprintf("%s:%s %016x", ip.Address, ip.Port, p.NodeID)
	p.send = NewParcelChannel(p.net.conf.ChannelCapacity)
	p.error = make(chan error, 10)
	p.IsIncoming = incoming
	p.conn = con
	p.Connected = time.Now()

	p.lastPeerRequest = time.Now()
	p.lastPeerSend = time.Now()
	p.peerShareAsk = true
	if !p.deliver(request) {
		tmplogger.Error("failed to deliver handshake to peermanager")
		return false
	}

	go p.sendLoop()
	go p.readLoop()

	return true
}

// Stop disconnects the peer from its active connection
func (p *Peer) Stop(andRemove bool) {
	p.stopper.Do(func() {
		p.logger.Debug("Stopping peer")
		sc := p.send

		p.send = nil
		p.error = nil

		p.stop <- true
		p.stopDelivery <- true

		if p.conn != nil {
			p.conn.Close()
		}

		close(sc)

		if andRemove {
			p.net.peerManager.peerDisconnect <- p
		}
	})
}

func (p *Peer) String() string {
	return p.Hash
}

func (p *Peer) Send(parcel *Parcel) {
	parcel.Header.PeerPort = p.net.conf.ListenPort
	parcel.Header.Network = p.net.conf.Network
	parcel.Header.Version = p.net.conf.ProtocolVersion
	p.send.Send(parcel)
}

func (p *Peer) quality(diff int32) int32 {
	p.metricsMtx.Lock()
	defer p.metricsMtx.Unlock()
	p.QualityScore += diff
	return p.QualityScore
}

func (p *Peer) readLoop() {
	defer p.conn.Close() // close connection on fatal error
	for {
		var message Parcel

		p.conn.SetReadDeadline(time.Now().Add(p.net.conf.ReadDeadline))
		err := p.decoder.Decode(&message)
		if err != nil {
			p.logger.WithError(err).Debug("connection error (readLoop)")
			p.Stop(false)
			return
		}

		p.metricsMtx.Lock()
		p.LastReceive = time.Now()
		p.QualityScore++
		p.ParcelsReceived++
		p.BytesReceived += uint64(len(message.Payload))
		p.metricsMtx.Unlock()

		message.Header.TargetPeer = p.Hash

		if !p.deliver(&message) {
			return
		}
	}
}

func (p *Peer) deliver(parcel *Parcel) bool {
	select {
	case p.net.peerParcel <- PeerParcel{Peer: p, Parcel: parcel}:
	case <-p.stopDelivery:
		return false
	}
	return true
}

// sendLoop listens to the Outgoing channel, pushing all data from there
// to the tcp connection
func (p *Peer) sendLoop() {
	defer p.conn.Close() // close connection on fatal error
	for {
		select {
		case <-p.stop:
			return
		case parcel := <-p.send:
			if parcel == nil {
				p.logger.Error("Received <nil> pointer")
				continue
			}

			p.conn.SetWriteDeadline(time.Now().Add(p.net.conf.WriteDeadline))
			err := p.encoder.Encode(parcel)
			if err != nil { // no error is recoverable
				p.logger.WithError(err).Debug("connection error (sendLoop)")
				p.Stop(false)
				return
			}

			p.metricsMtx.Lock()
			p.ParcelsSent++
			p.BytesSent += uint64(len(parcel.Payload))
			p.LastSend = time.Now()
			p.metricsMtx.Unlock()
		}
	}
}

func (p *Peer) GetMetrics() PeerMetrics {
	p.metricsMtx.RLock()
	defer p.metricsMtx.RUnlock()
	return PeerMetrics{
		Hash:            p.Hash,
		Connected:       p.Connected,
		LastReceive:     p.LastReceive,
		LastSend:        p.LastSend,
		BytesReceived:   p.BytesReceived,
		BytesSent:       p.BytesSent,
		ParcelsReceived: p.ParcelsReceived,
		ParcelsSent:     p.ParcelsSent,
		Incoming:        p.IsIncoming,
	}
}

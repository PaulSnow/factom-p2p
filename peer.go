// Copyright 2017 Factom Foundation
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

package p2p

import (
	"encoding/gob"
	"fmt"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

var peerLogger = packageLogger.WithField("subpack", "peer")

// Peer is a representation of another node in the network
//
// Peers exist in two forms: Temporary peers and regular peers
type Peer struct {
	net  *Network
	conn net.Conn
	//	endpoint  *Endpoint
	handshake Handshake

	// current state
	IsIncoming bool

	//	stateMutex             sync.RWMutex
	age     time.Time
	stopper sync.Once
	stop    chan bool

	lastPeerRequest time.Time
	lastPeerSend    time.Time
	send            ParcelChannel
	error           chan error
	disconnect      chan *Peer

	encoder *gob.Encoder // Wire format is gobs in this version, may switch to binary
	decoder *gob.Decoder // Wire format is gobs in this version, may switch to binary

	connectionAttempt      time.Time
	connectionAttemptCount uint
	LastReceive            time.Time // Keep track of how long ago we talked to the peer.
	LastSend               time.Time // Keep track of how long ago we talked to the peer.

	Port string
	Seed bool

	QualityScore int32  // 0 is neutral quality, negative is a bad peer.
	Address      string // Must be in form of x.x.x.x
	NodeID       uint64 // a nonce to distinguish multiple nodes behind one IP address
	Hash         string // This is more of a connection ID than hash right now.

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
		"address": p.Address,
		"Port":    p.Port,
		"version": p.handshake.Version,
	})
	p.age = time.Now()
	p.stop = make(chan bool, 1)

	p.logger.Debugf("Creating blank peer")
	return p
}

func (p *Peer) StartWithHandshake(ip IP, con net.Conn, incoming bool) bool {
	addr := ip.Address
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

	p.handshake = handshake
	p.Address = addr
	p.Port = handshake.Port
	p.NodeID = handshake.NodeID
	p.Hash = fmt.Sprintf("%s:%s %016x", p.Address, handshake.Port, handshake.NodeID)
	p.send = NewParcelChannel(p.net.conf.ChannelCapacity)
	p.error = make(chan error, 10)
	p.IsIncoming = incoming
	p.conn = con

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

		select {
		case p.stop <- true:
		default:
		}

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
	// send this parcel from this peer
	p.send.Send(parcel)
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

		p.LastReceive = time.Now()
		p.QualityScore++
		p.logger.Debugf("Received incoming parcel: %v", message)
		select {
		case p.net.peerParcel <- PeerParcel{Peer: p, Parcel: &message}:
		default:
			p.logger.Warn("Peer manager unable to handle load, dropping")
		}
	}
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
			p.LastSend = time.Now()
		}
	}
}

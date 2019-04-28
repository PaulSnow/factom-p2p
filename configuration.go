package p2p

import (
	"time"
)

const (
	BroadcastFlag     = "<BROADCAST>"
	FullBroadcastFlag = "<FULLBORADCAST>"
	RandomPeerFlag    = "<RANDOMPEER>"
)

// Configuration defines the behavior of the gossip network protocol
type Configuration struct {
	// Network is the NetworkID of the network to use, eg. MainNet, TestNet, etc
	Network NetworkID

	// NodeID is this node's id
	NodeID uint64
	// NodeName is the internal name of the node
	NodeName string

	// === Peer Management Settings ===
	// GossipConfiguration holds the configuration for the peer management system
	// PeerSaveInterval dictates how often peers should be saved to disk
	PeerSaveInterval time.Duration
	// PeerRequestInterval dictates how often neighbors should be asked for an
	// updated peer list
	PeerRequestInterval time.Duration
	// PeerReseedInterval dictates how often the seed file should be accessed
	// to check for changes
	PeerReseedInterval time.Duration
	// PeerIPLimit specifies the maximum amount of peers to accept from a single
	// ip address
	// 0 for unlimited
	PeerIPLimitIncoming uint
	PeerIPLimitOutgoing uint

	// Special is a list of special peers, separated by comma. If no port is specified, the entire
	// ip is considered special
	Special string

	// PeerFile is the filepath to the file to save peers
	PeerFile string
	// how often to save these
	PersistInterval time.Duration

	// === Gossip Behavior ===
	// Outgoing is the number of peers this node attempts to connect to
	Outgoing uint
	// Incoming is the number of incoming connections this node is willing to accept
	Incoming uint
	// PeerShareAmount is the number of peers we share
	PeerShareAmount     uint
	MinimumQualityScore int32

	// Fanout controls how many random peers are selected for propagating messages
	// Higher values increase fault tolerance but also increase network congestion
	Fanout uint

	// TrustedOnly indicates whether or not to connect only to trusted peers
	TrustedOnly bool
	// RefuseIncoming indicates whether or not to disallow all incoming connections
	// for example if you are behind a NAT
	RefuseIncoming bool
	// Refuse unknown peers from connecting but allow special peers
	RefuseUnknown bool
	// SeedURL is the URL of the remote seed file
	SeedURL string // URL to a source of peer info

	// === Connection Settings ===

	// BindIP is the ip address to bind to for listening and connecting
	//
	// leave blank to bind to all
	BindIP string
	// ListenPort is the port to listen to incoming tcp connections on
	ListenPort string
	// ListenLimit is the lockout period of accepting connections from a single
	// ip after having a successful connection from that ip
	ListenLimit time.Duration

	// PingInterval dictates the maximum amount of time a connection can be
	// silent (no writes) before sending a Ping
	PingInterval time.Duration

	// PeerAgeLimit dictates how long a peer can fail to connect before being considered dead
	PeerAgeLimit time.Duration
	// RedialInterval dictates how long to wait between connection attempts
	RedialInterval time.Duration
	// RedialReset dictates after how long we should try to reconnect again
	RedialReset time.Duration
	// RedialAttempts is the number of redial attempts to make before considering
	// a connection unreachable
	RedialAttempts uint
	// DisconnectLock dictates how long the peer manager should wait for an incoming peer
	// to reconnect before considering dialing to them
	DisconnectLock time.Duration

	// ManualBan is the duration to ban an address for when banned manually
	ManualBan time.Duration
	// AutoBan is the duration to ban an address for when their qualityscore drops too low
	AutoBan time.Duration

	// HandshakeDeadline is the maximum acceptable time for an incoming conneciton
	// to send the first parcel after connecting
	HandshakeTimeout time.Duration
	DialTimeout      time.Duration

	// ReadDeadline is the maximum acceptable time to read a single parcel
	// if a connection takes longer, it is disconnected
	ReadDeadline time.Duration

	// WriteDeadline is the maximum acceptable time to send a single parcel
	// if a connection takes longer, it is disconnected
	WriteDeadline time.Duration

	ProtocolVersion uint16
	// ProtocolVersionMinimum is the earliest version this package supports
	ProtocolVersionMinimum uint16

	ChannelCapacity uint

	LogPath  string // Path for logs
	LogLevel string // Logging level
}

// DefaultP2PConfiguration returns a network configuration with base values
// These should be overwritten with command line and config parameters
func DefaultP2PConfiguration() (c Configuration) {
	c.Network = MainNet
	c.NodeID = 0
	c.NodeName = "FNode0"

	c.PeerSaveInterval = time.Second * 30
	c.PeerRequestInterval = time.Minute * 3
	c.PeerReseedInterval = time.Hour * 4
	c.PeerIPLimitIncoming = 0
	c.PeerIPLimitOutgoing = 0
	c.ManualBan = time.Hour * 24 * 7 // a week
	c.AutoBan = time.Hour * 24 * 7   // a week

	c.PeerFile = ""
	c.PersistInterval = time.Minute * 15

	c.Outgoing = 32
	c.Incoming = 150
	c.Fanout = 16
	c.PeerShareAmount = 4 * c.Outgoing // legacy math
	c.MinimumQualityScore = 20

	c.TrustedOnly = false
	c.RefuseIncoming = false

	c.BindIP = "" // bind to all
	c.ListenPort = "8108"
	c.ListenLimit = time.Second
	c.PingInterval = time.Second * 15
	c.PeerAgeLimit = time.Hour * 48 // 2 days
	c.RedialInterval = time.Second * 20
	c.RedialReset = time.Hour * 12
	c.RedialAttempts = 5
	c.DisconnectLock = time.Minute * 3 // RedialInterval * RedialAttempts + 80 seconds

	c.ReadDeadline = time.Minute * 5      // high enough to accomodate large packets
	c.WriteDeadline = time.Minute * 5     // but fail eventually
	c.HandshakeTimeout = time.Second * 10 // can be quite low
	c.DialTimeout = time.Second * 10      // can be quite low

	c.ProtocolVersion = 9
	c.ProtocolVersionMinimum = 9

	c.ChannelCapacity = 5000

	return
}

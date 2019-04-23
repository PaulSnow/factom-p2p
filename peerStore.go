package p2p

import "sync"

// [p.hash] = peer
type PeerStore struct {
	mtx       sync.RWMutex
	peers     map[string]*Peer // hash -> peer
	connected map[string]int   // address -> count
	curSlice  []*Peer
	Incoming  int
	Outgoing  int
}

func NewPeerStore() *PeerStore {
	ps := new(PeerStore)
	ps.peers = make(map[string]*Peer)
	ps.connected = make(map[string]int)
	return ps
}

func (ps *PeerStore) Replace(p *Peer) *Peer {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	old := ps.peers[p.Hash]
	ps.curSlice = nil
	ps.peers[p.Hash] = p
	ps.connected[p.IP.Address]++

	if p.IsIncoming {
		ps.Incoming++
	} else {
		ps.Outgoing++
	}

	if old != nil {
		ps.connected[p.IP.Address]--
		if old.IsIncoming {
			ps.Incoming--
		} else {
			ps.Outgoing--
		}
	}
	return old
}

func (ps *PeerStore) Remove(p *Peer) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	if old, ok := ps.peers[p.Hash]; ok && old == p { // pointer comparison
		ps.connected[p.IP.Address]--
		if old.IsIncoming {
			ps.Incoming--
		} else {
			ps.Outgoing--
		}
		ps.curSlice = nil
		delete(ps.peers, p.Hash)
	}
}

func (ps *PeerStore) Total() int {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()
	return ps.Incoming + ps.Outgoing
}

func (ps *PeerStore) Get(hash string) *Peer {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()
	return ps.peers[hash]
}

func (ps *PeerStore) IsConnected(addr string) bool {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()
	return ps.connected[addr] > 0
}

func (ps *PeerStore) Count(addr string) int {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()
	return ps.connected[addr]
}

func (ps *PeerStore) Slice() []*Peer {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()

	if ps.curSlice != nil {
		return ps.curSlice
	}
	r := make([]*Peer, 0)
	for _, p := range ps.peers {
		r = append(r, p)
	}
	ps.curSlice = r
	return r
}

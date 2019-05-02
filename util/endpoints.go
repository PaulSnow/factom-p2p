package util

import (
	"encoding/json"
	"sync"
	"time"
)

// Endpoints is a collection of known ip addresses in the network.
// Aka the partial peer view.
// Endpoints are unique and there can be only one for a given IP
type Endpoints struct {
	Ends map[string]endpoint  `json:"endpoints"`
	Bans map[string]time.Time `json:"bans"`
	mtx  sync.RWMutex
	ips  []IP
}

type endpoint struct {
	IP     IP                   `json:"ip"`
	Seen   time.Time            `json:"seen"`
	Source map[string]time.Time `json:"source"`
	lock   time.Time
}

// NewEndPoints creates an empty endpoint holder
func NewEndpoints() *Endpoints {
	epm := new(Endpoints)
	epm.Ends = make(map[string]endpoint)
	epm.Bans = make(map[string]time.Time)
	return epm
}

func (epm *Endpoints) Total() int {
	return len(epm.Ends)
}

// Register an IP in the system along with the source of where it's from
func (epm *Endpoints) Register(ip IP, source string) {
	epm.mtx.Lock()
	defer epm.mtx.Unlock()
	ep := epm.Ends[ip.String()]
	if ep.Source == nil {
		ep.Source = make(map[string]time.Time)
	}
	ep.Seen = time.Now()
	ep.Source[source] = time.Now()
	ep.IP = ip
	epm.Ends[ip.String()] = ep
	epm.ips = nil
}

// Refresh updates the last time the endpoint had activity
func (epm *Endpoints) Refresh(ip IP) {
	epm.mtx.Lock()
	defer epm.mtx.Unlock()
	if ep, ok := epm.Ends[ip.String()]; ok {
		ep.Seen = time.Now()
		epm.Ends[ip.String()] = ep
	}
}

// Deregister removes an endpoint from the store
func (epm *Endpoints) Deregister(ip IP) {
	epm.mtx.Lock()
	defer epm.mtx.Unlock()
	delete(epm.Ends, ip.String())
	epm.ips = nil
}

// Ban all endpoints with a given ip address until a certain time
func (epm *Endpoints) Ban(addr string, t time.Time) {
	epm.mtx.Lock()
	defer epm.mtx.Unlock()
	for i, ep := range epm.Ends {
		if ep.IP.Address == addr {
			delete(epm.Ends, i)
		}
	}
	epm.Bans[addr] = t
	epm.ips = nil
}

// Banned checks if an ip address is banned
func (epm *Endpoints) Banned(addr string) bool {
	return time.Now().Before(epm.Bans[addr])
}

// LastSeen returns the time of the last activity
func (epm *Endpoints) LastSeen(ip IP) time.Time {
	epm.mtx.RLock()
	defer epm.mtx.RUnlock()
	return epm.Ends[ip.String()].Seen
}

// Lock an endpoint for a specific duration
func (epm *Endpoints) Lock(ip IP, dur time.Duration) {
	epm.mtx.Lock()
	defer epm.mtx.Unlock()
	if ep, ok := epm.Ends[ip.String()]; ok {
		ep.lock = time.Now().Add(dur)
		epm.Ends[ip.String()] = ep
	}
}

// Unlock an endpoint again
func (epm *Endpoints) Unlock(ip IP) {
	epm.mtx.Lock()
	defer epm.mtx.Unlock()
	if ep, ok := epm.Ends[ip.String()]; ok {
		ep.lock = time.Time{}
		epm.Ends[ip.String()] = ep
	}
}

// IsLocked checks if an endpoint is locked
func (epm *Endpoints) IsLocked(ip IP) bool {
	epm.mtx.RLock()
	defer epm.mtx.RUnlock()
	return time.Now().Before(epm.Ends[ip.String()].lock)
}

// IPs returns a concurrency safe slice of the current endpoints.
//
func (epm *Endpoints) IPs() []IP {
	epm.mtx.RLock()
	defer epm.mtx.RUnlock()

	if epm.ips != nil || len(epm.Ends) == 0 {
		return epm.ips
	}

	for _, ep := range epm.Ends {
		epm.ips = append(epm.ips, ep.IP)
	}
	return epm.ips
}

func (epm *Endpoints) Cleanup(cutoff time.Duration) uint {
	removed := uint(0)
	for addr, ep := range epm.Ends {
		if time.Since(ep.Seen) > cutoff {
			delete(epm.Ends, addr)
			removed++
		}
	}
	for addr, ban := range epm.Bans {
		if ban.Before(time.Now()) {
			delete(epm.Bans, addr)
		}
	}
	epm.ips = nil
	return removed
}

func (epm *Endpoints) Persist(cutoff time.Duration) ([]byte, error) {
	epm.mtx.RLock()
	defer epm.mtx.RUnlock()
	epm.Cleanup(cutoff)
	return json.Marshal(epm)
}
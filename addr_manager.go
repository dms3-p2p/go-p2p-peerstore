package peerstore

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/dms3-p2p/go-p2p-peer"
	"github.com/dms3-p2p/go-p2p-peerstore/addr"
	ma "github.com/dms3-mft/go-multiaddr"
)

var (

	// TempAddrTTL is the ttl used for a short lived address
	TempAddrTTL = time.Second * 10

	// ProviderAddrTTL is the TTL of an address we've received from a provider.
	// This is also a temporary address, but lasts longer. After this expires,
	// the records we return will require an extra lookup.
	ProviderAddrTTL = time.Minute * 10

	// RecentlyConnectedAddrTTL is used when we recently connected to a peer.
	// It means that we are reasonably certain of the peer's address.
	RecentlyConnectedAddrTTL = time.Minute * 10

	// OwnObservedAddrTTL is used for our own external addresses observed by peers.
	OwnObservedAddrTTL = time.Minute * 10
)

// Permanent TTLs (distinct so we can distinguish between them, constant as they
// are, in fact, permanent)
const (
	// PermanentAddrTTL is the ttl for a "permanent address" (e.g. bootstrap nodes).
	PermanentAddrTTL = math.MaxInt64 - iota

	// ConnectedAddrTTL is the ttl used for the addresses of a peer to whom
	// we're connected directly. This is basically permanent, as we will
	// clear them + re-add under a TempAddrTTL after disconnecting.
	ConnectedAddrTTL
)

type expiringAddr struct {
	Addr    ma.Multiaddr
	TTL     time.Duration
	Expires time.Time
}

func (e *expiringAddr) ExpiredBy(t time.Time) bool {
	return t.After(e.Expires)
}

type addrSlice []expiringAddr

// AddrManager manages addresses.
// The zero-value is ready to be used.
type AddrManager struct {
	addrmu sync.Mutex // guards addrs
	addrs  map[peer.ID]addrSlice

	subManager *AddrSubManager
}

// ensures the AddrManager is initialized.
// So we can use the zero value.
func (mgr *AddrManager) init() {
	if mgr.addrs == nil {
		mgr.addrs = make(map[peer.ID]addrSlice)
	}
	if mgr.subManager == nil {
		mgr.subManager = NewAddrSubManager()
	}
}

func (mgr *AddrManager) Peers() []peer.ID {
	mgr.addrmu.Lock()
	defer mgr.addrmu.Unlock()
	if mgr.addrs == nil {
		return nil
	}

	pids := make([]peer.ID, 0, len(mgr.addrs))
	for pid := range mgr.addrs {
		pids = append(pids, pid)
	}
	return pids
}

// AddAddr calls AddAddrs(p, []ma.Multiaddr{addr}, ttl)
func (mgr *AddrManager) AddAddr(p peer.ID, addr ma.Multiaddr, ttl time.Duration) {
	mgr.AddAddrs(p, []ma.Multiaddr{addr}, ttl)
}

// AddAddrs gives AddrManager addresses to use, with a given ttl
// (time-to-live), after which the address is no longer valid.
// If the manager has a longer TTL, the operation is a no-op for that address
func (mgr *AddrManager) AddAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration) {
	mgr.addrmu.Lock()
	defer mgr.addrmu.Unlock()

	// if ttl is zero, exit. nothing to do.
	if ttl <= 0 {
		return
	}

	// so zero value can be used
	mgr.init()

	oldAddrs := mgr.addrs[p]
	amap := make(map[string]expiringAddr, len(oldAddrs))
	for _, ea := range oldAddrs {
		amap[string(ea.Addr.Bytes())] = ea
	}

	// only expand ttls
	exp := time.Now().Add(ttl)
	for _, addr := range addrs {
		if addr == nil {
			log.Warningf("was passed nil multiaddr for %s", p)
			continue
		}

		addrstr := string(addr.Bytes())
		a, found := amap[addrstr]
		if !found || exp.After(a.Expires) {
			amap[addrstr] = expiringAddr{Addr: addr, Expires: exp, TTL: ttl}

			mgr.subManager.BroadcastAddr(p, addr)
		}
	}
	newAddrs := make([]expiringAddr, 0, len(amap))
	for _, ea := range amap {
		newAddrs = append(newAddrs, ea)
	}
	mgr.addrs[p] = newAddrs
}

// SetAddr calls mgr.SetAddrs(p, addr, ttl)
func (mgr *AddrManager) SetAddr(p peer.ID, addr ma.Multiaddr, ttl time.Duration) {
	mgr.SetAddrs(p, []ma.Multiaddr{addr}, ttl)
}

// SetAddrs sets the ttl on addresses. This clears any TTL there previously.
// This is used when we receive the best estimate of the validity of an address.
func (mgr *AddrManager) SetAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration) {
	mgr.addrmu.Lock()
	defer mgr.addrmu.Unlock()

	// so zero value can be used
	mgr.init()

	oldAddrs := mgr.addrs[p]
	amap := make(map[string]expiringAddr, len(oldAddrs))
	for _, ea := range oldAddrs {
		amap[string(ea.Addr.Bytes())] = ea
	}

	exp := time.Now().Add(ttl)
	for _, addr := range addrs {
		if addr == nil {
			log.Warningf("was passed nil multiaddr for %s", p)
			continue
		}
		// re-set all of them for new ttl.
		addrs := string(addr.Bytes())

		if ttl > 0 {
			amap[addrs] = expiringAddr{Addr: addr, Expires: exp, TTL: ttl}

			mgr.subManager.BroadcastAddr(p, addr)
		} else {
			delete(amap, addrs)
		}
	}
	newAddrs := make([]expiringAddr, 0, len(amap))
	for _, ea := range amap {
		newAddrs = append(newAddrs, ea)
	}
	mgr.addrs[p] = newAddrs
}

// UpdateAddrs updates the addresses associated with the given peer that have
// the given oldTTL to have the given newTTL.
func (mgr *AddrManager) UpdateAddrs(p peer.ID, oldTTL time.Duration, newTTL time.Duration) {
	mgr.addrmu.Lock()
	defer mgr.addrmu.Unlock()

	if mgr.addrs == nil {
		return
	}

	addrs, found := mgr.addrs[p]
	if !found {
		return
	}

	exp := time.Now().Add(newTTL)
	for i := range addrs {
		aexp := &addrs[i]
		if oldTTL == aexp.TTL {
			aexp.TTL = newTTL
			aexp.Expires = exp
		}
	}
}

// Addresses returns all known (and valid) addresses for a given
func (mgr *AddrManager) Addrs(p peer.ID) []ma.Multiaddr {
	mgr.addrmu.Lock()
	defer mgr.addrmu.Unlock()

	// not initialized? nothing to give.
	if mgr.addrs == nil {
		return nil
	}

	maddrs, found := mgr.addrs[p]
	if !found {
		return nil
	}

	now := time.Now()
	good := make([]ma.Multiaddr, 0, len(maddrs))
	cleaned := make([]expiringAddr, 0, len(maddrs))
	for _, m := range maddrs {
		if !m.ExpiredBy(now) {
			cleaned = append(cleaned, m)
			good = append(good, m.Addr)
		}
	}

	// clean up the expired ones.
	if len(cleaned) == 0 {
		delete(mgr.addrs, p)
	} else {
		mgr.addrs[p] = cleaned
	}
	return good
}

// ClearAddrs removes all previously stored addresses
func (mgr *AddrManager) ClearAddrs(p peer.ID) {
	mgr.addrmu.Lock()
	defer mgr.addrmu.Unlock()
	mgr.init()

	delete(mgr.addrs, p)
}

// AddrStream returns a channel on which all new addresses discovered for a
// given peer ID will be published.
func (mgr *AddrManager) AddrStream(ctx context.Context, p peer.ID) <-chan ma.Multiaddr {
	mgr.addrmu.Lock()
	defer mgr.addrmu.Unlock()
	mgr.init()

	baseaddrslice := mgr.addrs[p]
	initial := make([]ma.Multiaddr, 0, len(baseaddrslice))
	for _, a := range baseaddrslice {
		initial = append(initial, a.Addr)
	}

	return mgr.subManager.AddrStream(ctx, p, initial)
}

// An abstracted, pub-sub manager for address streams. Extracted from
// AddrManager in order to support additional implementations.
type AddrSubManager struct {
	mu   sync.RWMutex
	subs map[peer.ID][]*addrSub
}

// NewAddrSubManager initializes an AddrSubManager.
func NewAddrSubManager() *AddrSubManager {
	return &AddrSubManager{
		subs: make(map[peer.ID][]*addrSub),
	}
}

// Used internally by the address stream coroutine to remove a subscription
// from the manager.
func (mgr *AddrSubManager) removeSub(p peer.ID, s *addrSub) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	subs := mgr.subs[p]
	if len(subs) == 1 {
		if subs[0] != s {
			return
		}
		delete(mgr.subs, p)
		return
	}
	for i, v := range subs {
		if v == s {
			subs[i] = subs[len(subs)-1]
			subs[len(subs)-1] = nil
			mgr.subs[p] = subs[:len(subs)-1]
			return
		}
	}
}

// BroadcastAddr broadcasts a new address to all subscribed streams.
func (mgr *AddrSubManager) BroadcastAddr(p peer.ID, addr ma.Multiaddr) {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	if subs, ok := mgr.subs[p]; ok {
		for _, sub := range subs {
			sub.pubAddr(addr)
		}
	}
}

// AddrStream creates a new subscription for a given peer ID, pre-populating the
// channel with any addresses we might already have on file.
func (mgr *AddrSubManager) AddrStream(ctx context.Context, p peer.ID, initial []ma.Multiaddr) <-chan ma.Multiaddr {
	sub := &addrSub{pubch: make(chan ma.Multiaddr), ctx: ctx}

	out := make(chan ma.Multiaddr)

	mgr.mu.Lock()
	if _, ok := mgr.subs[p]; ok {
		mgr.subs[p] = append(mgr.subs[p], sub)
	} else {
		mgr.subs[p] = []*addrSub{sub}
	}
	mgr.mu.Unlock()

	sort.Sort(addr.AddrList(initial))

	go func(buffer []ma.Multiaddr) {
		defer close(out)

		sent := make(map[string]bool, len(buffer))
		var outch chan ma.Multiaddr

		for _, a := range buffer {
			sent[string(a.Bytes())] = true
		}

		var next ma.Multiaddr
		if len(buffer) > 0 {
			next = buffer[0]
			buffer = buffer[1:]
			outch = out
		}

		for {
			select {
			case outch <- next:
				if len(buffer) > 0 {
					next = buffer[0]
					buffer = buffer[1:]
				} else {
					outch = nil
					next = nil
				}
			case naddr := <-sub.pubch:
				if sent[string(naddr.Bytes())] {
					continue
				}

				sent[string(naddr.Bytes())] = true
				if next == nil {
					next = naddr
					outch = out
				} else {
					buffer = append(buffer, naddr)
				}
			case <-ctx.Done():
				mgr.removeSub(p, sub)
				return
			}
		}

	}(initial)

	return out
}

type addrSub struct {
	pubch  chan ma.Multiaddr
	lk     sync.Mutex
	buffer []ma.Multiaddr
	ctx    context.Context
}

func (s *addrSub) pubAddr(a ma.Multiaddr) {
	select {
	case s.pubch <- a:
	case <-s.ctx.Done():
	}
}

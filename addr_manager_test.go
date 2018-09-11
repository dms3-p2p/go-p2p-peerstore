package peerstore

import (
	"context"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/dms3-fs/go-datastore"
	"github.com/dms3-fs/go-ds-badger"
	"github.com/dms3-p2p/go-p2p-peer"
	ma "github.com/dms3-mft/go-multiaddr"
)

func IDS(t *testing.T, ids string) peer.ID {
	t.Helper()
	id, err := peer.IDB58Decode(ids)
	if err != nil {
		t.Fatalf("id %q is bad: %s", ids, err)
	}
	return id
}

func MA(t *testing.T, m string) ma.Multiaddr {
	t.Helper()
	maddr, err := ma.NewMultiaddr(m)
	if err != nil {
		t.Fatal(err)
	}
	return maddr
}

func testHas(t *testing.T, exp, act []ma.Multiaddr) {
	t.Helper()
	if len(exp) != len(act) {
		t.Fatalf("lengths not the same. expected %d, got %d\n", len(exp), len(act))
	}

	for _, a := range exp {
		found := false

		for _, b := range act {
			if a.Equal(b) {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("expected address %s not found", a)
		}
	}
}

func setupBadgerDatastore(t testing.TB) (datastore.Batching, func()) {
	dataPath, err := ioutil.TempDir(os.TempDir(), "badger")
	if err != nil {
		t.Fatal(err)
	}
	ds, err := badger.NewDatastore(dataPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	closer := func() {
		ds.Close()
		os.RemoveAll(dataPath)
	}
	return ds, closer
}

func setupDatastoreAddrManager(t *testing.T) (*DatastoreAddrManager, func()) {
	ds, closeDB := setupBadgerDatastore(t)
	mgr, err := NewDatastoreAddrManager(context.Background(), ds, 100*time.Microsecond)
	if err != nil {
		t.Fatal(err)
	}
	closer := func() {
		mgr.Stop()
		closeDB()
	}
	return mgr, closer
}

func runTestWithAddrManagers(t *testing.T, test func(*testing.T, AddrBook)) {
	t.Log("AddrManager")
	mgr1 := &AddrManager{}
	test(t, mgr1)

	t.Log("DatastoreAddrManager")
	mgr2, closer2 := setupDatastoreAddrManager(t)
	defer closer2()
	test(t, mgr2)
}

func testAddresses(t *testing.T, m AddrBook) {
	id1 := IDS(t, "QmcNstKuwBBoVTpSCSDrwzjgrRcaYXK833Psuz2EMHwyQN")
	id2 := IDS(t, "QmRmPL3FDZKE3Qiwv1RosLdwdvbvg17b2hB39QPScgWKKZ")
	id3 := IDS(t, "QmPhi7vBsChP7sjRoZGgg7bcKqF6MmCcQwvRbDte8aJ6Kn")
	id4 := IDS(t, "QmPhi7vBsChP7sjRoZGgg7bcKqF6MmCcQwvRbDte8aJ5Kn")
	id5 := IDS(t, "QmPhi7vBsChP7sjRoZGgg7bcKqF6MmCcQwvRbDte8aJ5Km")

	ma11 := MA(t, "/ip4/1.2.3.1/tcp/1111")
	ma21 := MA(t, "/ip4/2.2.3.2/tcp/1111")
	ma22 := MA(t, "/ip4/2.2.3.2/tcp/2222")
	ma31 := MA(t, "/ip4/3.2.3.3/tcp/1111")
	ma32 := MA(t, "/ip4/3.2.3.3/tcp/2222")
	ma33 := MA(t, "/ip4/3.2.3.3/tcp/3333")
	ma41 := MA(t, "/ip4/4.2.3.3/tcp/1111")
	ma42 := MA(t, "/ip4/4.2.3.3/tcp/2222")
	ma43 := MA(t, "/ip4/4.2.3.3/tcp/3333")
	ma44 := MA(t, "/ip4/4.2.3.3/tcp/4444")
	ma51 := MA(t, "/ip4/5.2.3.3/tcp/1111")
	ma52 := MA(t, "/ip4/5.2.3.3/tcp/2222")
	ma53 := MA(t, "/ip4/5.2.3.3/tcp/3333")
	ma54 := MA(t, "/ip4/5.2.3.3/tcp/4444")
	ma55 := MA(t, "/ip4/5.2.3.3/tcp/5555")

	ttl := time.Hour
	m.AddAddr(id1, ma11, ttl)

	m.AddAddrs(id2, []ma.Multiaddr{ma21, ma22}, ttl)
	m.AddAddrs(id2, []ma.Multiaddr{ma21, ma22}, ttl) // idempotency

	m.AddAddr(id3, ma31, ttl)
	m.AddAddr(id3, ma32, ttl)
	m.AddAddr(id3, ma33, ttl)
	m.AddAddr(id3, ma33, ttl) // idempotency
	m.AddAddr(id3, ma33, ttl)

	m.AddAddrs(id4, []ma.Multiaddr{ma41, ma42, ma43, ma44}, ttl) // multiple

	m.AddAddrs(id5, []ma.Multiaddr{ma21, ma22}, ttl)             // clearing
	m.AddAddrs(id5, []ma.Multiaddr{ma41, ma42, ma43, ma44}, ttl) // clearing
	m.ClearAddrs(id5)
	m.AddAddrs(id5, []ma.Multiaddr{ma51, ma52, ma53, ma54, ma55}, ttl) // clearing

	// test the Addresses return value
	testHas(t, []ma.Multiaddr{ma11}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma21, ma22}, m.Addrs(id2))
	testHas(t, []ma.Multiaddr{ma31, ma32, ma33}, m.Addrs(id3))
	testHas(t, []ma.Multiaddr{ma41, ma42, ma43, ma44}, m.Addrs(id4))
	testHas(t, []ma.Multiaddr{ma51, ma52, ma53, ma54, ma55}, m.Addrs(id5))
}

func TestAddresses(t *testing.T) {
	runTestWithAddrManagers(t, testAddresses)
}

func testAddressesExpire(t *testing.T, m AddrBook) {
	id1 := IDS(t, "QmcNstKuwBBoVTpSCSDrwzjgrRcaYXK833Psuz2EMHwyQN")
	id2 := IDS(t, "QmcNstKuwBBoVTpSCSDrwzjgrRcaYXK833Psuz2EMHwyQM")
	ma11 := MA(t, "/ip4/1.2.3.1/tcp/1111")
	ma12 := MA(t, "/ip4/2.2.3.2/tcp/2222")
	ma13 := MA(t, "/ip4/3.2.3.3/tcp/3333")
	ma24 := MA(t, "/ip4/4.2.3.3/tcp/4444")
	ma25 := MA(t, "/ip4/5.2.3.3/tcp/5555")

	m.AddAddr(id1, ma11, time.Hour)
	m.AddAddr(id1, ma12, time.Hour)
	m.AddAddr(id1, ma13, time.Hour)
	m.AddAddr(id2, ma24, time.Hour)
	m.AddAddr(id2, ma25, time.Hour)

	testHas(t, []ma.Multiaddr{ma11, ma12, ma13}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma24, ma25}, m.Addrs(id2))

	m.SetAddr(id1, ma11, 2*time.Hour)
	m.SetAddr(id1, ma12, 2*time.Hour)
	m.SetAddr(id1, ma13, 2*time.Hour)
	m.SetAddr(id2, ma24, 2*time.Hour)
	m.SetAddr(id2, ma25, 2*time.Hour)

	testHas(t, []ma.Multiaddr{ma11, ma12, ma13}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma24, ma25}, m.Addrs(id2))

	m.SetAddr(id1, ma11, time.Millisecond)
	<-time.After(time.Millisecond * 2)
	testHas(t, []ma.Multiaddr{ma12, ma13}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma24, ma25}, m.Addrs(id2))

	m.SetAddr(id1, ma13, time.Millisecond)
	<-time.After(time.Millisecond * 2)
	testHas(t, []ma.Multiaddr{ma12}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma24, ma25}, m.Addrs(id2))

	m.SetAddr(id2, ma24, time.Millisecond)
	<-time.After(time.Millisecond * 2)
	testHas(t, []ma.Multiaddr{ma12}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma25}, m.Addrs(id2))

	m.SetAddr(id2, ma25, time.Millisecond)
	<-time.After(time.Millisecond * 2)
	testHas(t, []ma.Multiaddr{ma12}, m.Addrs(id1))
	testHas(t, nil, m.Addrs(id2))

	m.SetAddr(id1, ma12, time.Millisecond)
	<-time.After(time.Millisecond * 2)
	testHas(t, nil, m.Addrs(id1))
	testHas(t, nil, m.Addrs(id2))
}

func TestAddressesExpire(t *testing.T) {
	runTestWithAddrManagers(t, testAddressesExpire)
}

func testClearWorks(t *testing.T, m AddrBook) {
	id1 := IDS(t, "QmcNstKuwBBoVTpSCSDrwzjgrRcaYXK833Psuz2EMHwyQN")
	id2 := IDS(t, "QmcNstKuwBBoVTpSCSDrwzjgrRcaYXK833Psuz2EMHwyQM")
	ma11 := MA(t, "/ip4/1.2.3.1/tcp/1111")
	ma12 := MA(t, "/ip4/2.2.3.2/tcp/2222")
	ma13 := MA(t, "/ip4/3.2.3.3/tcp/3333")
	ma24 := MA(t, "/ip4/4.2.3.3/tcp/4444")
	ma25 := MA(t, "/ip4/5.2.3.3/tcp/5555")

	m.AddAddr(id1, ma11, time.Hour)
	m.AddAddr(id1, ma12, time.Hour)
	m.AddAddr(id1, ma13, time.Hour)
	m.AddAddr(id2, ma24, time.Hour)
	m.AddAddr(id2, ma25, time.Hour)

	testHas(t, []ma.Multiaddr{ma11, ma12, ma13}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma24, ma25}, m.Addrs(id2))

	m.ClearAddrs(id1)
	m.ClearAddrs(id2)

	testHas(t, nil, m.Addrs(id1))
	testHas(t, nil, m.Addrs(id2))
}

func TestClearWorks(t *testing.T) {
	runTestWithAddrManagers(t, testClearWorks)
}

func testSetNegativeTTLClears(t *testing.T, m AddrBook) {
	id1 := IDS(t, "QmcNstKuwBBoVTpSCSDrwzjgrRcaYXK833Psuz2EMHwyQN")
	ma11 := MA(t, "/ip4/1.2.3.1/tcp/1111")

	m.SetAddr(id1, ma11, time.Hour)

	testHas(t, []ma.Multiaddr{ma11}, m.Addrs(id1))

	m.SetAddr(id1, ma11, -1)

	testHas(t, nil, m.Addrs(id1))
}
func TestSetNegativeTTLClears(t *testing.T) {
	runTestWithAddrManagers(t, testSetNegativeTTLClears)
}

func testUpdateTTLs(t *testing.T, m AddrBook) {
	id1 := IDS(t, "QmcNstKuwBBoVTpSCSDrwzjgrRcaYXK833Psuz2EMHwyQN")
	id2 := IDS(t, "QmcNstKuwBBoVTpSCSDrwzjgrRcaYXK833Psuz2EMHwyQM")
	ma11 := MA(t, "/ip4/1.2.3.1/tcp/1111")
	ma12 := MA(t, "/ip4/1.2.3.1/tcp/1112")
	ma21 := MA(t, "/ip4/1.2.3.1/tcp/1121")
	ma22 := MA(t, "/ip4/1.2.3.1/tcp/1122")

	// Shouldn't panic.
	m.UpdateAddrs(id1, time.Hour, time.Minute)

	m.SetAddr(id1, ma11, time.Hour)
	m.SetAddr(id1, ma12, time.Minute)

	// Shouldn't panic.
	m.UpdateAddrs(id2, time.Hour, time.Minute)

	m.SetAddr(id2, ma21, time.Hour)
	m.SetAddr(id2, ma22, time.Minute)

	testHas(t, []ma.Multiaddr{ma11, ma12}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma21, ma22}, m.Addrs(id2))

	m.UpdateAddrs(id1, time.Hour, time.Second)

	testHas(t, []ma.Multiaddr{ma11, ma12}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma21, ma22}, m.Addrs(id2))

	time.Sleep(1200 * time.Millisecond)

	testHas(t, []ma.Multiaddr{ma12}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma21, ma22}, m.Addrs(id2))

	m.UpdateAddrs(id2, time.Hour, time.Second)

	testHas(t, []ma.Multiaddr{ma12}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma21, ma22}, m.Addrs(id2))

	time.Sleep(1200 * time.Millisecond)

	testHas(t, []ma.Multiaddr{ma12}, m.Addrs(id1))
	testHas(t, []ma.Multiaddr{ma22}, m.Addrs(id2))
}

func TestUpdateTTLs(t *testing.T) {
	runTestWithAddrManagers(t, testUpdateTTLs)
}

func testNilAddrsDontBreak(t *testing.T, m AddrBook) {
	id1 := IDS(t, "QmcNstKuwBBoVTpSCSDrwzjgrRcaYXK833Psuz2EMHwyQN")
	m.SetAddr(id1, nil, time.Hour)
	m.AddAddr(id1, nil, time.Hour)
}

func TestNilAddrsDontBreak(t *testing.T) {
	runTestWithAddrManagers(t, testNilAddrsDontBreak)
}

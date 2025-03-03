package migration_01_to_11

import (
	"image/color"
	"math/big"
	prand "math/rand"
	"net"
	"time"

	lnwire "github.com/ltcsuite/lnd/channeldb/migration/lnwire21"
	"github.com/ltcsuite/ltcd/btcec/v2"
	"github.com/ltcsuite/ltcd/btcec/v2/ecdsa"
)

var (
	testAddr = &net.TCPAddr{IP: (net.IP)([]byte{0xA, 0x0, 0x0, 0x1}),
		Port: 9000}
	anotherAddr, _ = net.ResolveTCPAddr("tcp",
		"[2001:db8:85a3:0:0:8a2e:370:7334]:80")
	testAddrs = []net.Addr{testAddr, anotherAddr}

	testR, _    = new(big.Int).SetString("63724406601629180062774974542967536251589935445068131219452686511677818569431", 10)
	testS, _    = new(big.Int).SetString("18801056069249825825291287104931333862866033135609736119018462340006816851118", 10)
	testRScalar = new(btcec.ModNScalar)
	testSScalar = new(btcec.ModNScalar)
	_           = testRScalar.SetByteSlice(testR.Bytes())
	_           = testSScalar.SetByteSlice(testS.Bytes())
	testSig     = ecdsa.NewSignature(testRScalar, testSScalar)

	testFeatures = lnwire.NewFeatureVector(nil, nil)
)

func createLightningNode(db *DB, priv *btcec.PrivateKey) (*LightningNode, error) {
	updateTime := prand.Int63()

	pub := priv.PubKey().SerializeCompressed()
	n := &LightningNode{
		HaveNodeAnnouncement: true,
		AuthSigBytes:         testSig.Serialize(),
		LastUpdate:           time.Unix(updateTime, 0),
		Color:                color.RGBA{1, 2, 3, 0},
		Alias:                "kek" + string(pub[:]),
		Features:             testFeatures,
		Addresses:            testAddrs,
		db:                   db,
	}
	copy(n.PubKeyBytes[:], priv.PubKey().SerializeCompressed())

	return n, nil
}

func createTestVertex(db *DB) (*LightningNode, error) {
	priv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	return createLightningNode(db, priv)
}

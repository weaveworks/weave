package router

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"time"
)

const (
	Protocol                = "weave"
	ProtocolMinVersion byte = 1
	ProtocolVersion    byte = 2
)

var (
	ProtocolBytes = []byte(Protocol)

	HeaderTimeout = 10 * time.Second

	ProtocolV1Features = []string{
		"ConnID",
		"Name",
		"NickName",
		"PeerNameFlavour",
		"UID",
	}

	ErrExpectedCrypto   = fmt.Errorf("Password specified, but peer requested an unencrypted connection")
	ErrExpectedNoCrypto = fmt.Errorf("No password specificed, but peer requested an encrypted connection")
)

// We don't need the full net.TCPConn to do the protocol intro.  This
// interface contains just the parts we do need, to support testing
type ProtocolIntroConn interface {
	// io.Reader
	Read(b []byte) (n int, err error)

	// io.Writer
	Write(b []byte) (n int, err error)

	// net.Conn's deadline methods
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

type ProtocolIntroParams struct {
	Features map[string]string
	Conn     ProtocolIntroConn
	Password []byte
	Outbound bool
}

type ProtocolIntroResults struct {
	Features   map[string]string
	Receiver   TCPReceiver
	Sender     TCPSender
	SessionKey *[32]byte
	Version    byte
}

func (params ProtocolIntroParams) DoIntro() (res ProtocolIntroResults, err error) {
	return params.doIntro(ProtocolVersion)
}

func (params ProtocolIntroParams) doIntro(maxVersion byte) (res ProtocolIntroResults, err error) {
	if err = params.Conn.SetDeadline(time.Now().Add(HeaderTimeout)); err != nil {
		return
	}

	if res.Version, err = params.exchangeProtocolHeader(maxVersion); err != nil {
		return
	}

	var pubKey, privKey *[32]byte
	if params.Password != nil {
		if pubKey, privKey, err = GenerateKeyPair(); err != nil {
			return
		}
	}

	if err = params.Conn.SetWriteDeadline(time.Time{}); err != nil {
		return
	}
	if err = params.Conn.SetReadDeadline(time.Now().Add(TCPHeartbeat * 2)); err != nil {
		return
	}

	switch res.Version {
	case 1:
		err = res.doIntroV1(params, pubKey, privKey)
	case 2:
		err = res.doIntroV2(params, pubKey, privKey)
	default:
		panic("unhandled protocol version")
	}

	return
}

func (params ProtocolIntroParams) exchangeProtocolHeader(maxVersion byte) (byte, error) {
	// Write in a separate goroutine to avoid the possibility of
	// deadlock.  The result channel is of size 1 so that the
	// goroutine does not linger even if we encounter an error on
	// the read side.
	sendHeader := append(ProtocolBytes, ProtocolMinVersion, maxVersion)
	writeDone := make(chan error, 1)
	go func() {
		_, err := params.Conn.Write(sendHeader)
		writeDone <- err
	}()

	header := make([]byte, len(ProtocolBytes)+2)
	if n, err := io.ReadFull(params.Conn, header); err != nil && n == 0 {
		return 0, fmt.Errorf("failed to receive remote protocol header: %s", err)
	} else if err != nil {
		return 0, fmt.Errorf("received incomplete remote protocol header (%d octets instead of %d): %v; error: %s",
			n, len(header), header[:n], err)
	}

	if !bytes.Equal(ProtocolBytes, header[:len(ProtocolBytes)]) {
		return 0, fmt.Errorf("remote protocol header not recognised: %v", header[:len(ProtocolBytes)])
	}

	theirMinVersion := header[len(ProtocolBytes)]
	minVersion := theirMinVersion
	if ProtocolMinVersion > minVersion {
		minVersion = ProtocolMinVersion
	}

	theirVersion := header[len(ProtocolBytes)+1]
	version := theirVersion
	if version > maxVersion {
		version = maxVersion
	}

	if minVersion > version {
		return 0, fmt.Errorf("remote version range [%d,%d] is incompatible with ours [%d,%d]",
			theirMinVersion, theirVersion,
			ProtocolMinVersion, maxVersion)
	}

	if err := <-writeDone; err != nil {
		return 0, err
	}

	return version, nil
}

// The V1 procotol consists of the protocol identification/version
// header, followed by a stream of gobified values.  The first value
// is the encoded features map (never encrypted).  The subsequent
// values are the messages on the connection (encrypted for an
// encrypted connection).  For an encrypted connection, the public key
// is passed in the "PublicKey" feature as a string of hex digits.
func (res *ProtocolIntroResults) doIntroV1(params ProtocolIntroParams, pubKey, privKey *[32]byte) error {
	features := filterV1Features(params.Features)
	if pubKey != nil {
		features["PublicKey"] = hex.EncodeToString(pubKey[:])
	}

	enc := gob.NewEncoder(params.Conn)
	dec := gob.NewDecoder(params.Conn)

	// Encode in a separate goroutine to avoid the possibility of
	// deadlock.  The result channel is of size 1 so that the
	// goroutine does not linger even if we encounter an error on
	// the read side.
	encodeDone := make(chan error, 1)
	go func() {
		encodeDone <- enc.Encode(features)
	}()

	if err := dec.Decode(&res.Features); err != nil {
		return err
	}

	if err := <-encodeDone; err != nil {
		return err
	}

	if pubKey == nil {
		if _, present := res.Features["PublicKey"]; present {
			return ErrExpectedNoCrypto
		}

		res.setupNoCrypto(enc, dec)
	} else {
		remotePubKeyStr, ok := res.Features["PublicKey"]
		if !ok {
			return ErrExpectedCrypto
		}

		remotePubKey, err := hex.DecodeString(remotePubKeyStr)
		if err != nil {
			return err
		}

		res.setupCrypto(params, enc, dec, remotePubKey, privKey)
	}

	res.Features = filterV1Features(res.Features)
	return nil
}

// In the V1 protocol, the intro fields are sent unencrypted.  So we
// restrict them to an established subset of fields that are assumed
// to be safe.
func filterV1Features(intro map[string]string) map[string]string {
	safe := make(map[string]string)
	for _, k := range ProtocolV1Features {
		if val, ok := intro[k]; ok {
			safe[k] = val
		}
	}

	return safe
}

// The V2 procotol consists of the protocol identification/version
// header, followed by:
//
// - A single "encryption flag" byte: 0 for no encryption, 1 for
// encryption.
//
// - When the connection is encrypted, 32 bytes follow containing the
// public key.
//
// - Then a stream of gobified values.
//
// The gobified values are the messages on the connection (encrypted
// for an encrypted connection).  The first message contains the
// encoded features map (so in contrast to V1, it will be encrypted on
// an encrypted connection).
func (res *ProtocolIntroResults) doIntroV2(params ProtocolIntroParams, pubKey, privKey *[32]byte) error {
	// Public key exchange
	var wbuf []byte
	if pubKey == nil {
		wbuf = []byte{0}
	} else {
		wbuf = make([]byte, 1+len(*pubKey))
		wbuf[0] = 1
		copy(wbuf[1:], (*pubKey)[:])
	}

	// Write in a separate goroutine to avoid the possibility of
	// deadlock.  The result channel is of size 1 so that the
	// goroutine does not linger even if we encounter an error on
	// the read side.
	writeDone := make(chan error, 1)
	go func() {
		_, err := params.Conn.Write(wbuf)
		writeDone <- err
	}()

	rbuf := make([]byte, 1)
	if _, err := io.ReadFull(params.Conn, rbuf); err != nil {
		return err
	}

	switch rbuf[0] {
	case 0:
		if pubKey != nil {
			return ErrExpectedCrypto
		}

		res.setupNoCrypto(gob.NewEncoder(params.Conn),
			gob.NewDecoder(params.Conn))

	case 1:
		if pubKey == nil {
			return ErrExpectedNoCrypto
		}

		rbuf = make([]byte, len(pubKey))
		if _, err := io.ReadFull(params.Conn, rbuf); err != nil {
			return err
		}

		res.setupCrypto(params, gob.NewEncoder(params.Conn),
			gob.NewDecoder(params.Conn), rbuf, privKey)

	default:
		return fmt.Errorf("Bad encryption flag %d", rbuf[0])
	}

	if err := <-writeDone; err != nil {
		return err
	}

	// Features exchange
	go func() {
		buf := new(bytes.Buffer)
		if err := gob.NewEncoder(buf).Encode(&params.Features); err != nil {
			writeDone <- err
			return
		}

		writeDone <- res.Sender.Send(buf.Bytes())
	}()

	rbuf, err := res.Receiver.Receive()
	if err != nil {
		return err
	}

	if err := gob.NewDecoder(bytes.NewReader(rbuf)).Decode(&res.Features); err != nil {
		return err
	}

	if err := <-writeDone; err != nil {
		return err
	}

	return nil
}

func (res *ProtocolIntroResults) setupNoCrypto(enc *gob.Encoder, dec *gob.Decoder) {
	res.Sender = NewSimpleTCPSender(enc)
	res.Receiver = NewSimpleTCPReceiver(dec)
}

func (res *ProtocolIntroResults) setupCrypto(params ProtocolIntroParams,
	enc *gob.Encoder, dec *gob.Decoder, remotePubKey []byte,
	privKey *[32]byte) {
	var remotePubKeyArr [32]byte
	copy(remotePubKeyArr[:], remotePubKey)
	res.SessionKey = FormSessionKey(&remotePubKeyArr, privKey, params.Password)
	res.Sender = NewEncryptedTCPSender(enc, res.SessionKey, params.Outbound)
	res.Receiver = NewEncryptedTCPReceiver(dec, res.SessionKey, params.Outbound)
}

type ProtocolTag byte

const (
	ProtocolHeartbeat ProtocolTag = iota
	ProtocolConnectionEstablished
	ProtocolFragmentationReceived
	ProtocolPMTUVerified
	ProtocolGossip
	ProtocolGossipUnicast
	ProtocolGossipBroadcast
)

type ProtocolMsg struct {
	tag ProtocolTag
	msg []byte
}

type ProtocolSender interface {
	SendProtocolMsg(m ProtocolMsg)
}

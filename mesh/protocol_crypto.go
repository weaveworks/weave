package mesh

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

const MaxTCPMsgSize = 10 * 1024 * 1024

func GenerateKeyPair() (publicKey, privateKey *[32]byte, err error) {
	return box.GenerateKey(rand.Reader)
}

func FormSessionKey(remotePublicKey, localPrivateKey *[32]byte, secretKey []byte) *[32]byte {
	var sharedKey [32]byte
	box.Precompute(&sharedKey, remotePublicKey, localPrivateKey)
	sharedKeySlice := sharedKey[:]
	sharedKeySlice = append(sharedKeySlice, secretKey...)
	sessionKey := sha256.Sum256(sharedKeySlice)
	return &sessionKey
}

// TCP Senders/Receivers

// The lowest 64 bits of the nonce contain the message sequence
// number. The top most bit indicates the connection polarity at the
// sender - '1' for outbound; the next indicates protocol type - '1'
// for TCP. The remaining 126 bits are zero. The polarity is needed so
// that the two ends of a connection do not use the same nonces; the
// protocol type so that the TCP connection nonces are distinct from
// nonces used by overlay connections, if they share the session key.
// This is a requirement of the NaCl Security Model; see
// http://nacl.cr.yp.to/box.html.
type TCPCryptoState struct {
	sessionKey *[32]byte
	nonce      [24]byte
	seqNo      uint64
}

func NewTCPCryptoState(sessionKey *[32]byte, outbound bool) *TCPCryptoState {
	s := &TCPCryptoState{sessionKey: sessionKey}
	if outbound {
		s.nonce[0] |= (1 << 7)
	}
	s.nonce[0] |= (1 << 6)
	return s
}

func (s *TCPCryptoState) advance() {
	s.seqNo++
	binary.BigEndian.PutUint64(s.nonce[16:24], s.seqNo)
}

type TCPSender interface {
	Send([]byte) error
}

type GobTCPSender struct {
	encoder *gob.Encoder
}

type LengthPrefixTCPSender struct {
	writer io.Writer
}

type EncryptedTCPSender struct {
	sync.RWMutex
	sender TCPSender
	state  *TCPCryptoState
}

func NewGobTCPSender(encoder *gob.Encoder) *GobTCPSender {
	return &GobTCPSender{encoder: encoder}
}

func (sender *GobTCPSender) Send(msg []byte) error {
	return sender.encoder.Encode(msg)
}

func NewLengthPrefixTCPSender(writer io.Writer) *LengthPrefixTCPSender {
	return &LengthPrefixTCPSender{writer: writer}
}

func (sender *LengthPrefixTCPSender) Send(msg []byte) error {
	l := len(msg)
	if l > MaxTCPMsgSize {
		return fmt.Errorf("outgoing message exceeds maximum size: %d > %d", l, MaxTCPMsgSize)
	}
	// We copy the message so we can send it in a single Write
	// operation, thus making this thread-safe without locking.
	prefixedMsg := make([]byte, 4+l)
	binary.BigEndian.PutUint32(prefixedMsg, uint32(l))
	copy(prefixedMsg[4:], msg)
	_, err := sender.writer.Write(prefixedMsg)
	return err
}

func NewEncryptedTCPSender(sender TCPSender, sessionKey *[32]byte, outbound bool) *EncryptedTCPSender {
	return &EncryptedTCPSender{sender: sender, state: NewTCPCryptoState(sessionKey, outbound)}
}

func (sender *EncryptedTCPSender) Send(msg []byte) error {
	sender.Lock()
	defer sender.Unlock()
	encodedMsg := secretbox.Seal(nil, msg, &sender.state.nonce, sender.state.sessionKey)
	sender.state.advance()
	return sender.sender.Send(encodedMsg)
}

type TCPReceiver interface {
	Receive() ([]byte, error)
}

type GobTCPReceiver struct {
	decoder *gob.Decoder
}

type LengthPrefixTCPReceiver struct {
	reader io.Reader
}

type EncryptedTCPReceiver struct {
	receiver TCPReceiver
	state    *TCPCryptoState
}

func NewGobTCPReceiver(decoder *gob.Decoder) *GobTCPReceiver {
	return &GobTCPReceiver{decoder: decoder}
}

func (receiver *GobTCPReceiver) Receive() ([]byte, error) {
	var msg []byte
	err := receiver.decoder.Decode(&msg)
	return msg, err
}

func NewLengthPrefixTCPReceiver(reader io.Reader) *LengthPrefixTCPReceiver {
	return &LengthPrefixTCPReceiver{reader: reader}
}

func (receiver *LengthPrefixTCPReceiver) Receive() ([]byte, error) {
	lenPrefix := make([]byte, 4)
	if _, err := io.ReadFull(receiver.reader, lenPrefix); err != nil {
		return nil, err
	}
	l := binary.BigEndian.Uint32(lenPrefix)
	if l > MaxTCPMsgSize {
		return nil, fmt.Errorf("incoming message exceeds maximum size: %d > %d", l, MaxTCPMsgSize)
	}
	msg := make([]byte, l)
	_, err := io.ReadFull(receiver.reader, msg)
	return msg, err
}

func NewEncryptedTCPReceiver(receiver TCPReceiver, sessionKey *[32]byte, outbound bool) *EncryptedTCPReceiver {
	return &EncryptedTCPReceiver{receiver: receiver, state: NewTCPCryptoState(sessionKey, !outbound)}
}

func (receiver *EncryptedTCPReceiver) Receive() ([]byte, error) {
	msg, err := receiver.receiver.Receive()
	if err != nil {
		return nil, err
	}

	decodedMsg, success := secretbox.Open(nil, msg, &receiver.state.nonce, receiver.state.sessionKey)
	if !success {
		return nil, fmt.Errorf("Unable to decrypt TCP msg")
	}

	receiver.state.advance()
	return decodedMsg, nil
}

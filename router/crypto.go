package router

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"sync"

	"github.com/andybalholm/go-bit"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

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

// Frame Encryptors

type Encryptor interface {
	FrameOverhead() int
	PacketOverhead() int
	IsEmpty() bool
	Bytes() ([]byte, error)
	AppendFrame(src []byte, dst []byte, frame []byte)
	TotalLen() int
}

type NonEncryptor struct {
	buf       []byte
	bufTail   []byte
	buffered  int
	prefixLen int
}

type NaClEncryptor struct {
	NonEncryptor
	buf        []byte
	prefixLen  int
	sessionKey *[32]byte
	nonce      [24]byte
	seqNo      uint64
	df         bool
}

func NewNonEncryptor(prefix []byte) *NonEncryptor {
	buf := make([]byte, MaxUDPPacketSize)
	prefixLen := copy(buf, prefix)
	return &NonEncryptor{
		buf:       buf,
		bufTail:   buf[prefixLen:],
		buffered:  prefixLen,
		prefixLen: prefixLen}
}

func (ne *NonEncryptor) PacketOverhead() int {
	return ne.prefixLen
}

func (ne *NonEncryptor) FrameOverhead() int {
	return NameSize + NameSize + 2
}

func (ne *NonEncryptor) IsEmpty() bool {
	return ne.buffered == ne.prefixLen
}

func (ne *NonEncryptor) Bytes() ([]byte, error) {
	buf := ne.buf[:ne.buffered]
	ne.buffered = ne.prefixLen
	ne.bufTail = ne.buf[ne.prefixLen:]
	return buf, nil
}

func (ne *NonEncryptor) AppendFrame(src []byte, dst []byte, frame []byte) {
	bufTail := ne.bufTail
	srcLen := copy(bufTail, src)
	bufTail = bufTail[srcLen:]
	dstLen := copy(bufTail, dst)
	bufTail = bufTail[dstLen:]
	binary.BigEndian.PutUint16(bufTail, uint16(len(frame)))
	bufTail = bufTail[2:]
	copy(bufTail, frame)
	ne.bufTail = bufTail[len(frame):]
	ne.buffered += srcLen + dstLen + 2 + len(frame)
}

func (ne *NonEncryptor) TotalLen() int {
	return ne.buffered
}

func NewNaClEncryptor(prefix []byte, sessionKey *[32]byte, outbound bool, df bool) *NaClEncryptor {
	buf := make([]byte, MaxUDPPacketSize)
	prefixLen := copy(buf, prefix)
	ne := &NaClEncryptor{
		NonEncryptor: *NewNonEncryptor([]byte{}),
		buf:          buf,
		prefixLen:    prefixLen,
		sessionKey:   sessionKey,
		df:           df}
	if outbound {
		ne.nonce[0] |= (1 << 7)
	}
	return ne
}

func (ne *NaClEncryptor) Bytes() ([]byte, error) {
	plaintext, err := ne.NonEncryptor.Bytes()
	if err != nil {
		return nil, err
	}
	// We carry the DF flag in the (unencrypted portion of the)
	// payload, rather than just extracting it from the packet headers
	// at the receiving end, since we do not trust routers not to mess
	// with headers. As we have different decryptors for non-DF and
	// DF, that would result in hard to track down packet drops due to
	// crypto errors.
	seqNoAndDF := ne.seqNo
	if ne.df {
		seqNoAndDF |= (1 << 63)
	}
	ciphertext := ne.buf
	binary.BigEndian.PutUint64(ciphertext[ne.prefixLen:], seqNoAndDF)
	binary.BigEndian.PutUint64(ne.nonce[16:24], seqNoAndDF)
	// Seal *appends* to ciphertext
	ciphertext = secretbox.Seal(ciphertext[:ne.prefixLen+8], plaintext, &ne.nonce, ne.sessionKey)
	ne.seqNo++
	return ciphertext, nil
}

func (ne *NaClEncryptor) PacketOverhead() int {
	return ne.prefixLen + 8 + secretbox.Overhead + ne.NonEncryptor.PacketOverhead()
}

func (ne *NaClEncryptor) TotalLen() int {
	return ne.PacketOverhead() + ne.NonEncryptor.TotalLen()
}

// Frame Decryptors

type FrameConsumer func(src []byte, dst []byte, frame []byte)

type Decryptor interface {
	IterateFrames([]byte, FrameConsumer) error
}

type NonDecryptor struct {
}

type NaClDecryptor struct {
	NonDecryptor
	sessionKey *[32]byte
	instance   *NaClDecryptorInstance
	instanceDF *NaClDecryptorInstance
}

type NaClDecryptorInstance struct {
	nonce               [24]byte
	currentWindow       uint64
	usedOffsets         *bit.Set
	previousUsedOffsets *bit.Set
}

func NewNaClDecryptorInstance(outbound bool) *NaClDecryptorInstance {
	di := &NaClDecryptorInstance{usedOffsets: bit.New()}
	if !outbound {
		di.nonce[0] |= (1 << 7)
	}
	return di
}

type PacketDecodingError struct {
	Desc string
}

func NewNonDecryptor() *NonDecryptor {
	return &NonDecryptor{}
}

func (nd *NonDecryptor) IterateFrames(packet []byte, consumer FrameConsumer) error {
	for len(packet) >= (2 + NameSize + NameSize) {
		srcNameByte := packet[:NameSize]
		packet = packet[NameSize:]
		dstNameByte := packet[:NameSize]
		packet = packet[NameSize:]
		length := binary.BigEndian.Uint16(packet[:2])
		packet = packet[2:]
		if len(packet) < int(length) {
			return PacketDecodingError{Desc: fmt.Sprintf("too short; expected frame of length %d, got %d", length, len(packet))}
		}
		frame := packet[:length]
		packet = packet[length:]
		consumer(srcNameByte, dstNameByte, frame)
	}
	if len(packet) > 0 {
		return PacketDecodingError{Desc: fmt.Sprintf("%d octets of trailing garbage", len(packet))}
	}
	return nil
}

func NewNaClDecryptor(sessionKey *[32]byte, outbound bool) *NaClDecryptor {
	return &NaClDecryptor{
		NonDecryptor: *NewNonDecryptor(),
		sessionKey:   sessionKey,
		instance:     NewNaClDecryptorInstance(outbound),
		instanceDF:   NewNaClDecryptorInstance(outbound)}
}

func (nd *NaClDecryptor) IterateFrames(packet []byte, consumer FrameConsumer) error {
	if len(packet) < 8 {
		return PacketDecodingError{Desc: fmt.Sprintf("encrypted UDP packet too short; expected length >= 8, got %d", len(packet))}
	}
	buf, success := nd.decrypt(packet)
	if !success {
		return PacketDecodingError{Desc: fmt.Sprint("UDP packet decryption failed")}
	}
	return nd.NonDecryptor.IterateFrames(buf, consumer)
}

func (nd *NaClDecryptor) decrypt(buf []byte) ([]byte, bool) {
	seqNoAndDF := binary.BigEndian.Uint64(buf[:8])
	df := (seqNoAndDF & (1 << 63)) != 0
	seqNo := seqNoAndDF & ((1 << 63) - 1)
	var di *NaClDecryptorInstance
	if df {
		di = nd.instanceDF
	} else {
		di = nd.instance
	}
	binary.BigEndian.PutUint64(di.nonce[16:24], seqNoAndDF)
	result, success := secretbox.Open(nil, buf[8:], &di.nonce, nd.sessionKey)
	if !success {
		return nil, false
	}
	// Drop duplicates. We do this *after* decryption since we must
	// not advance our state unless decryption succeeded. Doing so
	// would open an easy attack vector where an adversary could
	// inject a packet with a sequence number of (1 << 63) - 1,
	// causing all subsequent genuine packets to get dropped.
	offset, usedOffsets := di.advanceState(seqNo)
	if usedOffsets == nil || usedOffsets.Contains(offset) {
		// We have detected a possible replay attack, but it is
		// possible we may have just received a very old packet, or
		// duplication may have occurred in the network. So let's just
		// drop the packet silently.
		return nil, true
	}
	usedOffsets.Add(offset)
	return result, success
}

// We record seen message sequence numbers in a sliding window of
// 2*WindowSize which slides in WindowSize increments. This allows us
// to process out-of-order delivery within the window, while
// accurately discarding duplicates. By contrast any messages with
// sequence numbers below the window are discarded as potential
// duplicates.
//
// There are two sets, corresponding to the lower and upper half of
// the window. We slide the window s.t. that 2nd set always contains
// the highest seen sequence number. We do this regardless of how far
// ahead of the current window that sequence number might be, so we
// can cope with large gaps resulting from packet loss.

const (
	WindowSize = 20 // bits
)

func (di *NaClDecryptorInstance) advanceState(seqNo uint64) (int, *bit.Set) {
	var (
		offset = int(seqNo & ((1 << WindowSize) - 1))
		window = seqNo >> WindowSize
	)
	switch delta := int64(window - di.currentWindow); {
	case delta < -1:
		return offset, nil
	case delta == -1:
		return offset, di.previousUsedOffsets
	default:
		return offset, di.usedOffsets
	case delta == +1:
		di.currentWindow = window
		di.previousUsedOffsets = di.usedOffsets
		di.usedOffsets = bit.New()
		return offset, di.usedOffsets
	case delta > +1:
		di.currentWindow = window
		di.previousUsedOffsets = bit.New()
		di.usedOffsets = bit.New()
		return offset, di.usedOffsets
	}
}

// TCP Senders/Receivers

// The lowest 64 bits of the nonce contain the message sequence
// number. The top most bit indicates the connection polarity at the
// sender - '1' for outbound; the next indicates protocol type - '1'
// for TCP. The remaining 126 bits are zero. The polarity is needed so
// that the two ends of a connection do not use the same nonces; the
// protocol type so that the TCP and UDP sender nonces are disjoint.
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
	// Ensure that TCP and UDP sequences are disjoint
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

type SimpleTCPSender struct {
	encoder *gob.Encoder
}

type EncryptedTCPSender struct {
	SimpleTCPSender
	sync.RWMutex
	state *TCPCryptoState
}

func NewSimpleTCPSender(encoder *gob.Encoder) *SimpleTCPSender {
	return &SimpleTCPSender{encoder: encoder}
}

func (sender *SimpleTCPSender) Send(msg []byte) error {
	return sender.encoder.Encode(msg)
}

func NewEncryptedTCPSender(encoder *gob.Encoder, sessionKey *[32]byte, outbound bool) *EncryptedTCPSender {
	return &EncryptedTCPSender{
		SimpleTCPSender: *NewSimpleTCPSender(encoder),
		state:           NewTCPCryptoState(sessionKey, outbound)}
}

func (sender *EncryptedTCPSender) Send(msg []byte) error {
	sender.Lock()
	defer sender.Unlock()
	encodedMsg := secretbox.Seal(nil, msg, &sender.state.nonce, sender.state.sessionKey)
	sender.state.advance()
	return sender.SimpleTCPSender.Send(encodedMsg)
}

type TCPReceiver interface {
	Receive() ([]byte, error)
}

type SimpleTCPReceiver struct {
	decoder *gob.Decoder
}

type EncryptedTCPReceiver struct {
	SimpleTCPReceiver
	state *TCPCryptoState
}

func NewSimpleTCPReceiver(decoder *gob.Decoder) *SimpleTCPReceiver {
	return &SimpleTCPReceiver{decoder: decoder}
}

func (receiver *SimpleTCPReceiver) Receive() ([]byte, error) {
	var msg []byte
	err := receiver.decoder.Decode(&msg)
	return msg, err
}

func NewEncryptedTCPReceiver(decoder *gob.Decoder, sessionKey *[32]byte, outbound bool) *EncryptedTCPReceiver {
	return &EncryptedTCPReceiver{
		SimpleTCPReceiver: *NewSimpleTCPReceiver(decoder),
		state:             NewTCPCryptoState(sessionKey, !outbound)}
}

func (receiver *EncryptedTCPReceiver) Receive() ([]byte, error) {
	msg, err := receiver.SimpleTCPReceiver.Receive()
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

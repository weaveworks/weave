package router

import (
	"code.google.com/p/go-bit/bit"
	"code.google.com/p/go.crypto/nacl/box"
	"code.google.com/p/go.crypto/nacl/secretbox"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"log"
	"sync"
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

func GenerateRandomNonce() ([24]byte, error) {
	var nonce [24]byte
	n, err := rand.Read(nonce[:])
	if err != nil {
		return nonce, err
	}
	if n != 24 {
		return nonce, fmt.Errorf("Did not read enough - wanted 24, got %v", n)
	}
	return nonce, nil
}

func SetNonceLow15Bits(nonce *[24]byte, offset uint16) {
	// ensure top bit of offset is 0
	offset = offset & ((1 << 15) - 1)
	// grab top bit of nonce[22:24] (and clear out lower bits)
	nonceBits := binary.BigEndian.Uint16(nonce[22:24]) & (1 << 15)
	// Big endian => the MSB is stored at the *lowest* address. So
	// that top bit in nonce[22] should stay as the top bit in
	// nonce[22]
	binary.BigEndian.PutUint16(nonce[22:24], nonceBits+offset)
}

// Nonce encoding/decoding

func EncodeNonce(df bool) (*[24]byte, []byte, error) {
	nonce, err := GenerateRandomNonce()
	if err != nil {
		return nil, []byte{}, err
	}
	// wipe out lowest 15 bits, but encode the df right at the bottom
	flags := uint16(0)
	if df {
		flags |= 1
	}
	SetNonceLow15Bits(&nonce, flags)
	// NB: need to make a copy since callers may modify the array
	return &nonce, Concat(nonce[:]), nil
}

func DecodeNonce(msg []byte) (bool, *[24]byte) {
	flags := uint16(msg[23])
	df := 0 != (flags & 1)
	var nonce [24]byte
	// upper bound is exclusive so this avoids copying the flags
	copy(nonce[:], msg[:23])
	return df, &nonce
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
	offset     uint16
	nonce      *[24]byte
	nonceChan  chan *[24]byte
	prefixLen  int
	sessionKey *[32]byte
	sendNonce  func([]byte)
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

func NewNaClEncryptor(prefix []byte, sessionKey *[32]byte, sendNonce func([]byte), df bool) *NaClEncryptor {
	buf := make([]byte, MaxUDPPacketSize)
	prefixLen := copy(buf, prefix)
	return &NaClEncryptor{
		NonEncryptor: *NewNonEncryptor([]byte{}),
		buf:          buf,
		offset:       0,
		nonce:        nil,
		nonceChan:    make(chan *[24]byte, ChannelSize),
		prefixLen:    prefixLen,
		sessionKey:   sessionKey,
		sendNonce:    sendNonce,
		df:           df}
}

func (ne *NaClEncryptor) Bytes() ([]byte, error) {
	plaintext, err := ne.NonEncryptor.Bytes()
	if err != nil {
		return nil, err
	}
	offsetFlags := ne.offset
	// We carry the DF flag in the (unencrypted portion of the)
	// payload, rather than just extracting it from the packet headers
	// at the receiving end, since we do not trust routers not to mess
	// with headers. As we have different decryptors for non-DF and
	// DF, that would result in hard to track down packet drops due to
	// crypto errors.
	if ne.df {
		offsetFlags |= (1 << 15)
	}
	ciphertext := ne.buf
	binary.BigEndian.PutUint16(ciphertext[ne.prefixLen:], offsetFlags)
	nonce := ne.nonce
	if nonce == nil {
		freshNonce, encodedNonce, err := EncodeNonce(ne.df)
		if err != nil {
			return nil, err
		}
		ne.sendNonce(encodedNonce)
		ne.nonce = freshNonce
		nonce = freshNonce
	}
	offset := ne.offset
	SetNonceLow15Bits(nonce, offset)
	// Seal *appends* to ciphertext
	ciphertext = secretbox.Seal(ciphertext[:ne.prefixLen+2], plaintext, nonce, ne.sessionKey)

	offset = (offset + 1) & ((1 << 15) - 1)
	if offset == 0 {
		// need a new nonce please
		ne.nonce = <-ne.nonceChan
	} else if offset == 1<<14 { // half way through range, send new nonce
		nonce, encodedNonce, err := EncodeNonce(ne.df)
		if err != nil {
			return nil, err
		}
		ne.nonceChan <- nonce
		ne.sendNonce(encodedNonce)
	}
	ne.offset = offset

	return ciphertext, nil
}

func (ne *NaClEncryptor) PacketOverhead() int {
	return ne.prefixLen + 2 + secretbox.Overhead + ne.NonEncryptor.PacketOverhead()
}

func (ne *NaClEncryptor) TotalLen() int {
	return ne.PacketOverhead() + ne.NonEncryptor.TotalLen()
}

// Frame Decryptors

type FrameConsumer func(src []byte, dst []byte, frame []byte)

type Decryptor interface {
	IterateFrames([]byte, FrameConsumer) error
	ReceiveNonce([]byte)
	Shutdown()
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
	nonce               *[24]byte
	previousNonce       *[24]byte
	usedOffsets         *bit.Set
	previousUsedOffsets *bit.Set
	highestOffsetSeen   uint16
	nonceChan           chan *[24]byte
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

func (nd *NonDecryptor) Shutdown() {
}

func (nd *NonDecryptor) ReceiveNonce(msg []byte) {
	log.Println("Received Nonce on non-encrypted channel. Ignoring.")
}

func NewNaClDecryptor(sessionKey *[32]byte) *NaClDecryptor {
	return &NaClDecryptor{
		NonDecryptor: *NewNonDecryptor(),
		sessionKey:   sessionKey,
		instance: &NaClDecryptorInstance{
			usedOffsets: bit.New(),
			nonceChan:   make(chan *[24]byte, ChannelSize)},
		instanceDF: &NaClDecryptorInstance{
			usedOffsets: bit.New(),
			nonceChan:   make(chan *[24]byte, ChannelSize)}}
}

func (nd *NaClDecryptor) Shutdown() {
	close(nd.instance.nonceChan)
	close(nd.instanceDF.nonceChan)
}

func (nd *NaClDecryptor) ReceiveNonce(msg []byte) {
	df, nonce := DecodeNonce(msg)
	if df {
		nd.instanceDF.nonceChan <- nonce
	} else {
		nd.instance.nonceChan <- nonce
	}
}

func (nd *NaClDecryptor) IterateFrames(packet []byte, consumer FrameConsumer) error {
	buf, err := nd.decrypt(packet)
	if err != nil {
		return PacketDecodingError{Desc: fmt.Sprint("decryption failed; ", err)}
	}
	return nd.NonDecryptor.IterateFrames(buf, consumer)
}

func (nd *NaClDecryptor) decrypt(buf []byte) ([]byte, error) {
	offset := binary.BigEndian.Uint16(buf[:2])
	df := (offset & (1 << 15)) != 0
	offsetNoFlags := offset & ((1 << 15) - 1)
	var di *NaClDecryptorInstance
	if df {
		di = nd.instanceDF
	} else {
		di = nd.instance
	}
	nonce, usedOffsets, err := di.advanceState(offsetNoFlags)
	if err != nil {
		return nil, err
	}
	offsetNoFlagsInt := int(offsetNoFlags)
	if usedOffsets.Contains(offsetNoFlagsInt) {
		return nil, fmt.Errorf("Suspected replay attack detected when decrypting UDP packet")
	}
	SetNonceLow15Bits(nonce, offsetNoFlags)
	result, success := secretbox.Open(nil, buf[2:], nonce, nd.sessionKey)
	if !success {
		return nil, fmt.Errorf("Unable to decrypt UDP packet")
	}
	usedOffsets.Add(offsetNoFlagsInt)
	return result, nil
}

func (di *NaClDecryptorInstance) advanceState(offsetNoFlags uint16) (*[24]byte, *bit.Set, error) {
	var ok bool
	if di.nonce == nil {
		if offsetNoFlags > (1 << 13) {
			// offset is already beyond the first quarter and it's the
			// first thing we've seen?! I don't think so.
			return nil, nil, fmt.Errorf("Unexpected offset when decrypting UDP packet")
		}
		di.nonce, ok = <-di.nonceChan
		if !ok {
			return nil, nil, fmt.Errorf("Nonce chan closed")
		}
		di.highestOffsetSeen = offsetNoFlags
	} else {
		highestOffsetSeen := di.highestOffsetSeen
		switch {
		case offsetNoFlags < (1<<13) && highestOffsetSeen > ((1<<14)+(1<<13)) &&
			(highestOffsetSeen-offsetNoFlags) > ((1<<14)+(1<<13)):
			// offset is in the first quarter, highestOffsetSeen is in
			// the top quarter and under a quarter behind us. We
			// interpret this as we need to move to the next nonce
			di.previousUsedOffsets = di.usedOffsets
			di.usedOffsets = bit.New()
			di.previousNonce = di.nonce
			di.nonce, ok = <-di.nonceChan
			if !ok {
				return nil, nil, fmt.Errorf("Nonce chan closed")
			}
			di.highestOffsetSeen = offsetNoFlags
		case offsetNoFlags > highestOffsetSeen &&
			(offsetNoFlags-highestOffsetSeen) < (1<<13):
			// offset is under a quarter above highestOffsetSeen. This
			// is ok - maybe some packet loss
			di.highestOffsetSeen = offsetNoFlags
		case offsetNoFlags <= highestOffsetSeen &&
			(highestOffsetSeen-offsetNoFlags) < (1<<13):
			// offset is within a quarter of the highest we've
			// seen. This is ok - just assuming some out-of-order
			// delivery.
		case highestOffsetSeen < (1<<13) && offsetNoFlags > ((1<<14)+(1<<13)) &&
			(offsetNoFlags-highestOffsetSeen) > ((1<<14)+(1<<13)):
			// offset is in the last quarter, highestOffsetSeen is in
			// the first quarter, and offset is under a quarter behind
			// us. This is ok - as above, just some out of order. But
			// here it means we're dealing with the previous nonce
			return di.previousNonce, di.previousUsedOffsets, nil
		default:
			return nil, nil, fmt.Errorf("Unexpected offset when decrypting UDP packet")
		}
	}
	return di.nonce, di.usedOffsets, nil
}

// TCP Senders/Receivers

// The lowest 64 bits of the nonce contain the message sequence
// number. The top most bit indicates the connection polarity at the
// sender - '1' for outbound. The remaining 127 bits are zero. The
// polarity is needed so that the two ends of a connection do not use
// the same nonces. This is a requirement of the NaCl Security Model;
// see http://nacl.cr.yp.to/box.html.
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
	Decode([]byte) ([]byte, error)
}

type SimpleTCPReceiver struct {
}

type EncryptedTCPReceiver struct {
	SimpleTCPReceiver
	state *TCPCryptoState
}

func NewSimpleTCPReceiver() *SimpleTCPReceiver {
	return &SimpleTCPReceiver{}
}

func (receiver *SimpleTCPReceiver) Decode(msg []byte) ([]byte, error) {
	return msg, nil
}

func NewEncryptedTCPReceiver(sessionKey *[32]byte, outbound bool) *EncryptedTCPReceiver {
	return &EncryptedTCPReceiver{
		SimpleTCPReceiver: *NewSimpleTCPReceiver(),
		state:             NewTCPCryptoState(sessionKey, !outbound)}
}

func (receiver *EncryptedTCPReceiver) Decode(msg []byte) ([]byte, error) {
	decodedMsg, success := secretbox.Open(nil, msg, &receiver.state.nonce, receiver.state.sessionKey)
	if !success {
		return nil, fmt.Errorf("Unable to decrypt TCP msg")
	}
	receiver.state.advance()
	return decodedMsg, nil
}

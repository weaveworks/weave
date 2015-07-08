package router

import (
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

type FieldValidator struct {
	fields map[string]string
	err    error
}

func NewFieldValidator(fields map[string]string) *FieldValidator {
	return &FieldValidator{fields, nil}
}

func (fv *FieldValidator) Value(fieldName string) (string, error) {
	if fv.err != nil {
		return "", fv.err
	}
	val, found := fv.fields[fieldName]
	if !found {
		fv.err = fmt.Errorf("Field %s is missing", fieldName)
		return "", fv.err
	}
	return val, nil
}

func (fv *FieldValidator) CheckEqual(fieldName, expectedValue string) error {
	val, err := fv.Value(fieldName)
	if err != nil {
		return err
	}
	if val != expectedValue {
		fv.err = fmt.Errorf("Field %s has wrong value; expected '%s', received '%s'", fieldName, expectedValue, val)
		return fv.err
	}
	return nil
}

func (fv *FieldValidator) Err() error {
	return fv.err
}

func (conn *LocalConnection) handshake() (TCPReceiver, *Peer, error) {
	// We do not need to worry about locking in here as at this point
	// the connection is not reachable by any go-routine other than
	// ourself. Only when we add this connection to the conn.local
	// peer will it be visible from multiple go-routines.

	conn.TCPConn.SetDeadline(time.Now().Add(HeaderTimeout))
	version, err := exchangeProtocolHeader(conn.TCPConn, conn.TCPConn)
	if err != nil {
		return nil, nil, err
	}
	conn.version = version
	conn.TCPConn.SetDeadline(time.Time{})
	conn.extendReadDeadline()

	localConnID := randUint64()
	usingPassword := conn.Router.UsingPassword()
	enc := gob.NewEncoder(conn.TCPConn)
	dec := gob.NewDecoder(conn.TCPConn)
	fv, private, err := conn.exchangeHandshake(localConnID, usingPassword, enc, dec)
	if err != nil {
		return nil, nil, err
	}

	nameStr, _ := fv.Value("Name")
	nickNameStr, _ := fv.Value("NickName")
	uidStr, _ := fv.Value("UID")
	remoteConnIDStr, _ := fv.Value("ConnID")
	if err := fv.Err(); err != nil {
		return nil, nil, err
	}

	name, err := PeerNameFromString(nameStr)
	if err != nil {
		return nil, nil, err
	}
	uid, err := ParsePeerUID(uidStr)
	if err != nil {
		return nil, nil, err
	}
	remoteConnID, err := strconv.ParseUint(remoteConnIDStr, 10, 64)
	if err != nil {
		return nil, nil, err
	}
	conn.uid = localConnID ^ remoteConnID

	var tcpReceiver TCPReceiver
	remotePublicStr, rpErr := fv.Value("PublicKey")
	if usingPassword {
		if rpErr != nil {
			return nil, nil, rpErr
		}
		remotePublicSlice, rpErr := hex.DecodeString(remotePublicStr)
		if rpErr != nil {
			return nil, nil, rpErr
		}
		remotePublic := [32]byte{}
		for idx, elem := range remotePublicSlice {
			remotePublic[idx] = elem
		}
		conn.SessionKey = FormSessionKey(&remotePublic, private, conn.Router.Password)
		conn.tcpSender = NewEncryptedTCPSender(enc, conn.SessionKey, conn.outbound)
		tcpReceiver = NewEncryptedTCPReceiver(dec, conn.SessionKey, conn.outbound)
		conn.Decryptor = NewNaClDecryptor(conn.SessionKey, conn.outbound)
	} else {
		if rpErr == nil {
			return nil, nil, fmt.Errorf("Remote network is encrypted. Password required.")
		}
		conn.tcpSender = NewSimpleTCPSender(enc)
		tcpReceiver = NewSimpleTCPReceiver(dec)
		conn.Decryptor = NewNonDecryptor()
	}

	return tcpReceiver, NewPeer(name, nickNameStr, uid, 0), nil
}

func (conn *LocalConnection) exchangeHandshake(localConnID uint64, usingPassword bool, enc *gob.Encoder, dec *gob.Decoder) (*FieldValidator, *[32]byte, error) {
	handshakeSend := map[string]string{
		"PeerNameFlavour": PeerNameFlavour,
		"Name":            conn.local.Name.String(),
		"NickName":        conn.local.NickName,
		"UID":             fmt.Sprint(conn.local.UID),
		"ConnID":          fmt.Sprint(localConnID)}
	handshakeRecv := map[string]string{}
	var public, private *[32]byte
	var err error
	if usingPassword {
		public, private, err = GenerateKeyPair()
		if err != nil {
			return nil, nil, err
		}
		handshakeSend["PublicKey"] = hex.EncodeToString(public[:])
	}
	enc.Encode(handshakeSend)
	if err = dec.Decode(&handshakeRecv); err != nil {
		return nil, nil, err
	}
	fv := NewFieldValidator(handshakeRecv)
	fv.CheckEqual("PeerNameFlavour", PeerNameFlavour)
	return fv, private, nil
}

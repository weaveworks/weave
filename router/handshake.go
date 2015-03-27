package router

import (
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"strconv"
)

type FieldValidator struct {
	fields map[string]string
	err    error
}

func NewFieldValidator(fields map[string]string) FieldValidator {
	return FieldValidator{fields, nil}
}

func (fv FieldValidator) Value(fieldName string) (string, error) {
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

func (fv FieldValidator) CheckEqual(fieldName, expectedValue string) error {
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

func (fv FieldValidator) Err() error {
	return fv.err
}

func (conn *LocalConnection) handshake(enc *gob.Encoder, dec *gob.Decoder, acceptNewPeer bool) error {
	// We do not need to worry about locking in here as at this point
	// the connection is not reachable by any go-routine other than
	// ourself. Only when we add this connection to the conn.local
	// peer will it be visible from multiple go-routines.

	conn.extendReadDeadline()

	localConnID := randUint64()
	versionStr := fmt.Sprint(ProtocolVersion)
	handshakeSend := map[string]string{
		"Protocol":        Protocol,
		"ProtocolVersion": versionStr,
		"PeerNameFlavour": PeerNameFlavour,
		"Name":            conn.local.Name.String(),
		"NickName":        conn.local.NickName,
		"UID":             fmt.Sprint(conn.local.UID),
		"ConnID":          fmt.Sprint(localConnID)}
	handshakeRecv := map[string]string{}

	usingPassword := conn.Router.UsingPassword()
	var public, private *[32]byte
	var err error
	if usingPassword {
		public, private, err = GenerateKeyPair()
		if err != nil {
			return err
		}
		handshakeSend["PublicKey"] = hex.EncodeToString(public[:])
	}
	enc.Encode(handshakeSend)

	err = dec.Decode(&handshakeRecv)
	if err != nil {
		return err
	}
	fv := NewFieldValidator(handshakeRecv)
	fv.CheckEqual("Protocol", Protocol)
	fv.CheckEqual("ProtocolVersion", versionStr)
	fv.CheckEqual("PeerNameFlavour", PeerNameFlavour)
	nameStr, _ := fv.Value("Name")
	nickNameStr, _ := fv.Value("NickName")
	uidStr, _ := fv.Value("UID")
	remoteConnIDStr, _ := fv.Value("ConnID")
	if err := fv.Err(); err != nil {
		return err
	}

	name, err := PeerNameFromString(nameStr)
	if err != nil {
		return err
	}
	if !acceptNewPeer {
		if _, found := conn.Router.Peers.Fetch(name); !found {
			return fmt.Errorf("Found unknown remote name: %s at %s", name, conn.remoteTCPAddr)
		}
	}
	if existingConn, found := conn.local.ConnectionTo(name); found && existingConn.Established() {
		return fmt.Errorf("Already have connection to %s at %s", name, existingConn.RemoteTCPAddr())
	}
	uid, err := strconv.ParseUint(uidStr, 10, 64)
	if err != nil {
		return err
	}
	remoteConnID, err := strconv.ParseUint(remoteConnIDStr, 10, 64)
	if err != nil {
		return err
	}
	conn.uid = localConnID ^ remoteConnID

	if usingPassword {
		remotePublicStr, rpErr := fv.Value("PublicKey")
		if rpErr != nil {
			return rpErr
		}
		remotePublicSlice, rpErr := hex.DecodeString(remotePublicStr)
		if rpErr != nil {
			return rpErr
		}
		remotePublic := [32]byte{}
		for idx, elem := range remotePublicSlice {
			remotePublic[idx] = elem
		}
		conn.SessionKey = FormSessionKey(&remotePublic, private, conn.Router.Password)
		conn.tcpSender = NewEncryptedTCPSender(enc, conn)
		conn.Decryptor = NewNaClDecryptor(conn)
	} else {
		if _, found := handshakeRecv["PublicKey"]; found {
			return fmt.Errorf("Remote network is encrypted. Password required.")
		}
		conn.tcpSender = NewSimpleTCPSender(enc)
		conn.Decryptor = NewNonDecryptor(conn)
	}

	toPeer := NewPeer(name, nickNameStr, uid, 0)
	toPeer = conn.Router.Peers.FetchWithDefault(toPeer)
	switch toPeer {
	case nil:
		return fmt.Errorf("Connection appears to be with different version of a peer we already know of")
	case conn.local:
		conn.remote = toPeer // have to do assigment here to ensure Shutdown releases ref count
		return fmt.Errorf("Cannot connect to ourself")
	default:
		conn.remote = toPeer
		return nil
	}
}

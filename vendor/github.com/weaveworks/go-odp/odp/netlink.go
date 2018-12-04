package odp

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"syscall"
)

func align(n int, a int) int {
	return (n + a - 1) & -a
}

type NetlinkSocket struct {
	fd   int
	addr *syscall.SockaddrNetlink
	buf  []byte
}

func OpenNetlinkSocket(protocol int) (*NetlinkSocket, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, protocol)
	if err != nil {
		return nil, err
	}

	success := false
	defer func() {
		if !success {
			syscall.Close(fd)
		}
	}()

	// It's fairly easy to provoke ENOBUFS from a netlink socket
	// receiving miss upcalls when every packet misses.  The
	// default socket buffer size is relatively small at 200KB,
	// and the default of /proc/sys/net/core/rmem_max means we
	// can't easily increase it.
	if err := syscall.SetsockoptInt(fd, SOL_NETLINK, syscall.NETLINK_NO_ENOBUFS, 1); err != nil {
		return nil, err
	}

	addr := syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}
	if err := syscall.Bind(fd, &addr); err != nil {
		return nil, err
	}

	localaddr, err := syscall.Getsockname(fd)
	if err != nil {
		return nil, err
	}

	nladdr, ok := localaddr.(*syscall.SockaddrNetlink)
	if !ok {
		return nil, fmt.Errorf("Expected netlink sockaddr, got %s", reflect.TypeOf(localaddr))
	}

	success = true
	return &NetlinkSocket{
		fd:   fd,
		addr: nladdr,

		// netlink messages can be bigger than this, but it
		// seems unlikely in practice, and this is similar to
		// the limit that the OVS userspace imposes.
		buf: make([]byte, 65536),
	}, nil
}

func (s *NetlinkSocket) PortId() uint32 {
	return s.addr.Pid
}

func (s *NetlinkSocket) Close() error {
	if s.fd < 0 {
		return nil
	}

	err := syscall.Close(s.fd)
	s.fd = -1
	return err
}

type NlMsgBuilder struct {
	buf []byte
}

func NewNlMsgBuilder(flags uint16, typ uint16) *NlMsgBuilder {
	buf := MakeAlignedByteSlice(syscall.NLMSG_HDRLEN)
	nlmsg := &NlMsgBuilder{buf: buf}
	h := nlMsghdrAt(buf, 0)
	h.Flags = flags
	h.Type = typ
	return nlmsg
}

// Expand the array underlying a slice to have capacity of at least l
func expand(buf []byte, l int) []byte {
	c := (cap(buf) + 1) * 3 / 2
	for l > c {
		c = (c + 1) * 3 / 2
	}
	new := MakeAlignedByteSliceCap(len(buf), c)
	copy(new, buf)
	return new
}

func (nlmsg *NlMsgBuilder) Align(a int) {
	l := align(len(nlmsg.buf), a)
	if l > cap(nlmsg.buf) {
		nlmsg.buf = expand(nlmsg.buf, l)
	}
	nlmsg.buf = nlmsg.buf[:l]
}

func (nlmsg *NlMsgBuilder) Grow(size uintptr) int {
	pos := len(nlmsg.buf)
	l := pos + int(size)
	if l > cap(nlmsg.buf) {
		nlmsg.buf = expand(nlmsg.buf, l)
	}
	nlmsg.buf = nlmsg.buf[:l]
	return pos
}

func (nlmsg *NlMsgBuilder) AlignGrow(a int, size uintptr) int {
	apos := align(len(nlmsg.buf), a)
	l := apos + int(size)
	if l > cap(nlmsg.buf) {
		nlmsg.buf = expand(nlmsg.buf, l)
	}
	nlmsg.buf = nlmsg.buf[:l]
	return apos
}

var nextSeqNo uint32

func (nlmsg *NlMsgBuilder) Finish() (res []byte, seq uint32) {
	h := nlMsghdrAt(nlmsg.buf, 0)
	h.Len = uint32(len(nlmsg.buf))
	seq = atomic.AddUint32(&nextSeqNo, 1)
	h.Seq = seq
	res = nlmsg.buf
	nlmsg.buf = nil
	return
}

func (nlmsg *NlMsgBuilder) PutAttr(typ uint16, gen func()) {
	pos := nlmsg.AlignGrow(syscall.NLA_ALIGNTO, syscall.SizeofNlAttr)
	gen()
	nla := nlAttrAt(nlmsg.buf, pos)
	nla.Type = typ
	nla.Len = uint16(len(nlmsg.buf) - pos)
}

func (nlmsg *NlMsgBuilder) PutNestedAttrs(typ uint16, gen func()) {
	nlmsg.PutAttr(typ, func() {
		gen()

		// The kernel nlattr parser expects the alignment
		// padding at the end of a nested attributes value to
		// be included in the length of the enclosing
		// attribute
		nlmsg.Align(syscall.NLA_ALIGNTO)
	})
}

func (nlmsg *NlMsgBuilder) PutEmptyAttr(typ uint16) {
	nlmsg.PutAttr(typ, func() {})
}

func (nlmsg *NlMsgBuilder) PutUint8Attr(typ uint16, val uint8) {
	nlmsg.PutAttr(typ, func() {
		pos := nlmsg.Grow(1)
		nlmsg.buf[pos] = val
	})
}

func (nlmsg *NlMsgBuilder) PutUint16Attr(typ uint16, val uint16) {
	nlmsg.PutAttr(typ, func() {
		pos := nlmsg.Grow(2)
		*uint16At(nlmsg.buf, pos) = val
	})
}

func (nlmsg *NlMsgBuilder) PutUint32Attr(typ uint16, val uint32) {
	nlmsg.PutAttr(typ, func() {
		pos := nlmsg.Grow(4)
		*uint32At(nlmsg.buf, pos) = val
	})
}

func (nlmsg *NlMsgBuilder) putStringZ(str string) {
	l := len(str)
	pos := nlmsg.Grow(uintptr(l) + 1)
	copy(nlmsg.buf[pos:], str)
	nlmsg.buf[pos+l] = 0
}

func (nlmsg *NlMsgBuilder) PutStringAttr(typ uint16, str string) {
	nlmsg.PutAttr(typ, func() { nlmsg.putStringZ(str) })
}

func (nlmsg *NlMsgBuilder) PutSliceAttr(typ uint16, data []byte) {
	nlmsg.PutAttr(typ, func() {
		pos := nlmsg.Grow(uintptr(len(data)))
		copy(nlmsg.buf[pos:], data)
	})
}

type NetlinkError syscall.Errno

func (err NetlinkError) Error() string {
	return fmt.Sprintf("netlink error response: %s", syscall.Errno(err))
}

type NlMsgParser struct {
	data []byte
	pos  int
}

func (nlmsg *NlMsgParser) Advance(size uintptr) error {
	if err := nlmsg.CheckAvailable(size); err != nil {
		return err
	}

	nlmsg.pos += int(size)
	return nil
}

func (nlmsg *NlMsgParser) AlignAdvance(a int, size uintptr) (int, error) {
	pos := align(nlmsg.pos, a)
	nlmsg.pos = pos
	if err := nlmsg.Advance(size); err != nil {
		return 0, err
	}

	return pos, nil
}

func (nlmsg *NlMsgParser) NlMsghdr() *syscall.NlMsghdr {
	return nlMsghdrAt(nlmsg.data, nlmsg.pos)
}

func (msg *NlMsgParser) nextNlMsg() (*NlMsgParser, error) {
	pos := msg.pos
	avail := len(msg.data) - pos
	if avail <= 0 {
		return nil, nil
	}

	if avail < syscall.SizeofNlMsghdr {
		return nil, fmt.Errorf("netlink message header truncated")
	}

	h := msg.NlMsghdr()
	if avail < int(h.Len) {
		return nil, fmt.Errorf("netlink message truncated (%d bytes available, %d expected)", avail, h.Len)
	}

	end := pos + int(h.Len)
	msg.pos = align(end, syscall.NLMSG_ALIGNTO)
	return &NlMsgParser{data: msg.data[:end], pos: pos}, nil
}

func (nlmsg *NlMsgParser) CheckAvailable(size uintptr) error {
	if nlmsg.pos+int(size) > len(nlmsg.data) {
		return fmt.Errorf("netlink message truncated")
	}

	return nil
}

func (nlmsg *NlMsgParser) checkHeader() error {
	// nextNlMsg ensures that there is an nlmsghdr-worth of data
	// present
	h := nlmsg.NlMsghdr()
	if h.Type == syscall.NLMSG_ERROR {
		nlerr := nlMsgerrAt(nlmsg.data, nlmsg.pos+syscall.NLMSG_HDRLEN)
		if nlerr.Error != 0 {
			return NetlinkError(-nlerr.Error)
		}

		// an error code of 0 means the error is an ack, so
		// return normally.
	}

	return nil
}

func (nlmsg *NlMsgParser) checkResponseHeader(expectedPortId uint32, expectedSeq uint32) (relevant bool, err error) {
	// nextNlMsg ensures that there is an nlmsghdr-worth of data
	// present
	h := nlmsg.NlMsghdr()
	if h.Pid != expectedPortId {
		return true, fmt.Errorf("netlink reply port id mismatch (got %d, expected %d)", h.Pid, expectedPortId)
	}

	if h.Seq != expectedSeq {
		// This doesn't necessarily indicate an error.  For
		// example, if an early requestMulti was interrupted
		// due to an error, we might still be getting its
		// response messages back that, and we should discard
		// them.  On the other hand, sequence number
		// mismatches might indicate bugs, so it is sometimes
		// nice to see them in development.
		fmt.Printf("netlink reply sequence number mismatch (got %d, expected %d)\n", h.Seq, expectedSeq)
		return false, nil
	}

	return true, nlmsg.checkHeader()
}

func (nlmsg *NlMsgParser) ExpectNlMsghdr(typ uint16) (*syscall.NlMsghdr, error) {
	h := nlmsg.NlMsghdr()

	if err := nlmsg.Advance(syscall.SizeofNlMsghdr); err != nil {
		return nil, err
	}

	if h.Type != typ {
		return nil, fmt.Errorf("netlink response has wrong type (got %d, expected %d)", h.Type, typ)
	}

	return h, nil
}

type Attrs map[uint16][]byte

func (attrs Attrs) Get(typ uint16, optional bool) ([]byte, error) {
	val, ok := attrs[typ]
	if !ok && !optional {
		return nil, fmt.Errorf("missing netlink attribute %d", typ)
	}

	return val, nil
}

func (attrs Attrs) GetFixedBytes(typ uint16, expect int, optional bool) ([]byte, error) {
	val, err := attrs.Get(typ, optional)
	if err != nil || val == nil {
		return nil, err
	}

	if len(val) != expect {
		return nil, fmt.Errorf("attribute %d has wrong length (got %d bytes, expected %d bytes)", typ, len(val), expect)
	}

	return val, nil
}

func (attrs Attrs) GetOptionalBytes(typ uint16, dest []byte) (bool, error) {
	val, err := attrs.GetFixedBytes(typ, len(dest), true)
	if err != nil || val == nil {
		return false, err
	}

	copy(dest, val)
	return true, nil
}

func (attrs Attrs) GetEmpty(typ uint16) (bool, error) {
	val, err := attrs.Get(typ, true)
	if err != nil || val == nil {
		return false, err
	}

	if len(val) != 0 {
		return false, fmt.Errorf("empty attribute %d has wrong length (%d bytes)", typ, len(val))
	}

	return true, nil
}

func (attrs Attrs) GetOptionalUint8(typ uint16) (uint8, bool, error) {
	val, err := attrs.Get(typ, true)
	if err != nil || val == nil {
		return 0, false, err
	}

	if len(val) != 1 {
		return 0, false, fmt.Errorf("uint8 attribute %d has wrong length (%d bytes)", typ, len(val))
	}

	return val[0], true, nil
}

func (attrs Attrs) getUint16(typ uint16, optional bool) (uint16, bool, error) {
	val, err := attrs.Get(typ, optional)
	if err != nil || val == nil {
		return 0, false, err
	}

	if len(val) != 2 {
		return 0, false, fmt.Errorf("uint16 attribute %d has wrong length (%d bytes)", typ, len(val))
	}

	return *uint16At(val, 0), true, nil
}

func (attrs Attrs) GetUint16(typ uint16) (uint16, error) {
	res, _, err := attrs.getUint16(typ, false)
	return res, err
}

func (attrs Attrs) GetOptionalUint16(typ uint16) (uint16, bool, error) {
	return attrs.getUint16(typ, true)
}

func (attrs Attrs) GetUint32(typ uint16) (uint32, error) {
	val, err := attrs.Get(typ, false)
	if err != nil {
		return 0, err
	}

	if len(val) != 4 {
		return 0, fmt.Errorf("uint32 attribute %d has wrong length (%d bytes)", typ, len(val))
	}

	return *uint32At(val, 0), nil
}

func (attrs Attrs) getUint64(typ uint16, optional bool) (uint64, bool, error) {
	val, err := attrs.Get(typ, optional)
	if err != nil || val == nil {
		return 0, false, err
	}

	if len(val) != 8 {
		return 0, false, fmt.Errorf("uint64 attribute %d has wrong length (%d bytes)", typ, len(val))
	}

	return *uint64At(val, 0), true, nil
}

func (attrs Attrs) GetUint64(typ uint16) (uint64, error) {
	res, _, err := attrs.getUint64(typ, false)
	return res, err
}

func (attrs Attrs) GetOptionalUint64(typ uint16) (uint64, bool, error) {
	return attrs.getUint64(typ, true)
}

func (attrs Attrs) GetString(typ uint16) (string, error) {
	val, err := attrs.Get(typ, false)
	if err != nil {
		return "", err
	}

	if len(val) == 0 {
		return "", fmt.Errorf("string attribute %d has zero length", typ)
	}

	if val[len(val)-1] != 0 {
		return "", fmt.Errorf("string attribute %d does not end with nul byte", typ)
	}

	return string(val[0 : len(val)-1]), nil
}

func (nlmsg *NlMsgParser) checkData(l uintptr, obj string) error {
	if nlmsg.pos+int(l) <= len(nlmsg.data) {
		return nil
	} else {
		return fmt.Errorf("truncated %s (have %d bytes, expected %d)", obj, len(nlmsg.data)-nlmsg.pos, l)
	}
}

func (nlmsg *NlMsgParser) parseAttrs(consumer func(uint16, []byte)) error {
	for {
		apos := align(nlmsg.pos, syscall.NLA_ALIGNTO)
		if len(nlmsg.data) <= apos {
			break
		}

		nlmsg.pos = apos

		if err := nlmsg.checkData(syscall.SizeofNlAttr, "netlink attribute"); err != nil {
			return err
		}

		nla := nlAttrAt(nlmsg.data, nlmsg.pos)
		if err := nlmsg.checkData(uintptr(nla.Len), "netlink attribute"); err != nil {
			return err
		}

		valpos := align(nlmsg.pos+syscall.SizeofNlAttr, syscall.NLA_ALIGNTO)
		consumer(nla.Type, nlmsg.data[valpos:nlmsg.pos+int(nla.Len)])
		nlmsg.pos += int(nla.Len)
	}

	return nil
}

func (nlmsg *NlMsgParser) TakeAttrs() (Attrs, error) {
	res := make(Attrs)
	err := nlmsg.parseAttrs(func(typ uint16, val []byte) {
		res[typ] = val
	})
	return res, err
}

func ParseNestedAttrs(data []byte) (Attrs, error) {
	parser := NlMsgParser{data: data, pos: 0}
	return parser.TakeAttrs()
}

func (attrs Attrs) GetNestedAttrs(typ uint16, optional bool) (Attrs, error) {
	val, err := attrs.Get(typ, optional)
	if val == nil {
		return nil, err
	}

	return ParseNestedAttrs(val)
}

// Usually we parse attributes into a map, but there are cases where
// attribute order matters.

type Attr struct {
	typ uint16
	val []byte
}

func (attrs Attrs) GetOrderedAttrs(typ uint16) ([]Attr, error) {
	val, err := attrs.Get(typ, false)
	if val == nil {
		return nil, err
	}

	parser := NlMsgParser{data: val, pos: 0}
	res := make([]Attr, 0)
	err = parser.parseAttrs(func(typ uint16, val []byte) {
		res = append(res, Attr{typ, val})
	})

	return res, err
}

func (s *NetlinkSocket) send(msg *NlMsgBuilder) (uint32, error) {
	sa := syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Pid:    0,
		Groups: 0,
	}

	data, seq := msg.Finish()
	return seq, syscall.Sendto(s.fd, data, 0, &sa)
}

func (s *NetlinkSocket) recv(peer uint32) (*NlMsgParser, error) {
	nr, from, err := syscall.Recvfrom(s.fd, s.buf, 0)
	if err != nil {
		return nil, err
	}

	buf := MakeAlignedByteSlice(nr)
	copy(buf, s.buf)

	switch nlfrom := from.(type) {
	case *syscall.SockaddrNetlink:
		if nlfrom.Pid != peer {
			return nil, fmt.Errorf("wrong netlink peer pid (expected %d, got %d)", peer, nlfrom.Pid)
		}

		return &NlMsgParser{data: buf, pos: 0}, nil

	default:
		return nil, fmt.Errorf("Expected netlink sockaddr, got %s", reflect.TypeOf(from))
	}
}

func (s *NetlinkSocket) Receive(consumer func(*NlMsgParser) (bool, error)) error {
	for {
		resp, err := s.recv(0)
		if err != nil {
			return err
		}

		msg, err := resp.nextNlMsg()
		if err != nil {
			return err
		}
		if msg == nil {
			return fmt.Errorf("netlink response message missing")
		}

		for {
			done, err := consumer(msg)
			if done || err != nil {
				return err
			}

			msg, err = resp.nextNlMsg()
			if err != nil {
				return err
			}
			if msg == nil {
				break
			}
		}
	}
}

// Some generic netlink operations always return a reply message (e.g
// *_GET), others don't by default (e.g. *_NEW).  In the latter case,
// NLM_F_ECHO forces a reply.  This is undocumented AFAICT.
const RequestFlags = syscall.NLM_F_REQUEST | syscall.NLM_F_ECHO

// Do a netlink request that yields a single response message.
func (s *NetlinkSocket) Request(req *NlMsgBuilder) (resp *NlMsgParser, err error) {
	seq, err := s.send(req)
	if err != nil {
		return nil, err
	}

	err = s.Receive(func(msg *NlMsgParser) (bool, error) {
		relevant, err := msg.checkResponseHeader(s.PortId(), seq)
		if relevant && err == nil {
			resp = msg
		}
		return true, err
	})
	return
}

const DumpFlags = syscall.NLM_F_DUMP | syscall.NLM_F_REQUEST

// Do a netlink request that yield multiple response messages.
func (s *NetlinkSocket) RequestMulti(req *NlMsgBuilder, consumer func(*NlMsgParser) error) error {
	seq, err := s.send(req)
	if err != nil {
		return err
	}

	return s.Receive(func(msg *NlMsgParser) (bool, error) {
		relevant, err := msg.checkResponseHeader(s.PortId(), seq)
		if !relevant || err != nil {
			return false, err
		}

		if msg.NlMsghdr().Type == syscall.NLMSG_DONE {
			return true, processNlMsgDone(msg)
		}

		err = consumer(msg)
		if err != nil {
			return true, err
		}

		return false, nil
	})
}

func processNlMsgDone(msg *NlMsgParser) error {
	err := msg.Advance(syscall.SizeofNlMsghdr)
	if err != nil {
		return err
	}

	err = msg.checkData(4, "NLMSG_DONE error code")
	if err != nil {
		return err
	}

	errno := *int32At(msg.data, msg.pos)
	if errno == 0 {
		return nil
	} else {
		return NetlinkError(-errno)
	}
}

type Consumer interface {
	Error(err error, stopped bool)
}

func (s *NetlinkSocket) consume(consumer Consumer, handler func(*NlMsgParser) error) {
	for {
		err := s.Receive(func(msg *NlMsgParser) (bool, error) {
			err := msg.checkHeader()
			if err == nil {
				err = handler(msg)
				if err == nil {
					return false, nil
				}
			}

			consumer.Error(err, false)
			return false, nil
		})

		if err != nil {
			consumer.Error(err, true)
			break
		}
	}
}

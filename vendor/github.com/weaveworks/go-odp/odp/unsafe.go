package odp

import (
	"syscall"
	"unsafe"
)

const ALIGN_BUFFERS = 8

// Normal slice or array allocations in golang do not appear to be
// guaranteed to be aligned (though in practice they are).  Unaligned
// access are slow on some architectures and blow up on others.  So
// this allocates a slice aligned to ALIGN_BUFFERS.
func MakeAlignedByteSliceCap(len int, cap int) []byte {
	b := make([]byte, cap+ALIGN_BUFFERS-1)
	off := int(uintptr(unsafe.Pointer(&b[0])) & (ALIGN_BUFFERS - 1))
	if off == 0 {
		// Already aligned
		return b[:len]
	} else {
		// Need to offset the slice to make it aligned
		off = ALIGN_BUFFERS - off
		return b[off : len+off]
	}
}

func MakeAlignedByteSlice(len int) []byte {
	return MakeAlignedByteSliceCap(len, len)
}

func uint16At(data []byte, pos int) *uint16 {
	return (*uint16)(unsafe.Pointer(&data[pos]))
}

func uint32At(data []byte, pos int) *uint32 {
	return (*uint32)(unsafe.Pointer(&data[pos]))
}

func int32At(data []byte, pos int) *int32 {
	return (*int32)(unsafe.Pointer(&data[pos]))
}

func uint64At(data []byte, pos int) *uint64 {
	return (*uint64)(unsafe.Pointer(&data[pos]))
}

func nlMsghdrAt(data []byte, pos int) *syscall.NlMsghdr {
	return (*syscall.NlMsghdr)(unsafe.Pointer(&data[pos]))
}

func nlAttrAt(data []byte, pos int) *syscall.NlAttr {
	return (*syscall.NlAttr)(unsafe.Pointer(&data[pos]))
}

func nlMsgerrAt(data []byte, pos int) *syscall.NlMsgerr {
	return (*syscall.NlMsgerr)(unsafe.Pointer(&data[pos]))
}

func genlMsghdrAt(data []byte, pos int) *GenlMsghdr {
	return (*GenlMsghdr)(unsafe.Pointer(&data[pos]))
}

func ovsHeaderAt(data []byte, pos int) *OvsHeader {
	return (*OvsHeader)(unsafe.Pointer(&data[pos]))
}

func ovsKeyEthernetAt(data []byte, pos int) *OvsKeyEthernet {
	return (*OvsKeyEthernet)(unsafe.Pointer(&data[pos]))
}

func ovsFlowStatsAt(data []byte, pos int) *OvsFlowStats {
	return (*OvsFlowStats)(unsafe.Pointer(&data[pos]))
}

func uint16FromBE(n uint16) uint16 {
	a := (*[2]byte)(unsafe.Pointer(&n))
	return uint16(a[0])<<8 + uint16(a[1])
}

func uint16ToBE(n uint16) uint16 {
	return uint16FromBE(n)
}

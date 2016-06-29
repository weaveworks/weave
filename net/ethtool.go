package net

import "fmt"
import "syscall"
import "unsafe"

const (
	SIOCETHTOOL     = 0x8946     // linux/sockios.h
	ETHTOOL_STXCSUM = 0x00000017 // linux/ethtool.h
	IFNAMSIZ        = 16         // linux/if.h
)

// linux/if.h 'struct ifreq'
type IFReqData struct {
	Name [IFNAMSIZ]byte
	Data uintptr
}

// linux/ethtool.h 'struct ethtool_value'
type EthtoolValue struct {
	Cmd  uint32
	Data uint32
}

// Disable TX checksum offload on specified interface
func EthtoolTXOff(name string) error {
	if len(name)+1 > IFNAMSIZ {
		return fmt.Errorf("name too long")
	}

	value := EthtoolValue{ETHTOOL_STXCSUM, 0}
	request := IFReqData{Data: uintptr(unsafe.Pointer(&value))}

	copy(request.Name[:], name)

	socket, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(socket)

	_, _, errno := syscall.RawSyscall(syscall.SYS_IOCTL,
		uintptr(socket),
		uintptr(SIOCETHTOOL),
		uintptr(unsafe.Pointer(&request)))

	if errno != 0 {
		return errno
	}

	return nil
}

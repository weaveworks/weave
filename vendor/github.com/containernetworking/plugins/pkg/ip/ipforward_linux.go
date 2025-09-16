// Copyright 2015 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ip

import (
	"bytes"
	"os"

	current "github.com/containernetworking/cni/pkg/types/100"
)

func EnableIP4Forward() error {
	return echo1("/proc/sys/net/ipv4/ip_forward")
}

func EnableIP6Forward() error {
	return echo1("/proc/sys/net/ipv6/conf/all/forwarding")
}

// EnableForward will enable forwarding for all configured
// address families
func EnableForward(ips []*current.IPConfig) error {
	v4 := false
	v6 := false

	for _, ip := range ips {
		isV4 := ip.Address.IP.To4() != nil
		if isV4 && !v4 {
			if err := EnableIP4Forward(); err != nil {
				return err
			}
			v4 = true
		} else if !isV4 && !v6 {
			if err := EnableIP6Forward(); err != nil {
				return err
			}
			v6 = true
		}
	}
	return nil
}

func echo1(f string) error {
	if content, err := os.ReadFile(f); err == nil {
		if bytes.Equal(bytes.TrimSpace(content), []byte("1")) {
			return nil
		}
	}
	return os.WriteFile(f, []byte("1"), 0644)
}

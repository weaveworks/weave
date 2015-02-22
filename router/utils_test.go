package router

import (
        wt "github.com/zettio/weave/testing"
        "testing"
        "net"
)

func TestIntMac(t *testing.T) {
    const known_mac = "01:23:45:67:89:ab"
    const known_int = uint64(1250999896491)
    var hw,_ = net.ParseMAC(known_mac)
    var actual = macint(hw)
    wt.AssertEqualuint64(t, actual, known_int, "MAC as int matches")
}

func TestMacInt(t *testing.T) {
    const known_mac = "01:23:45:67:89:ab"
    const known_int = uint64(1250999896491)
    var actual = intmac(known_int)
    wt.AssertEqualString(t, actual.String(), known_mac, "Int as MAC matches")
}

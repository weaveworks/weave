package nameserver

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "github.com/weaveworks/weave/common"
)

func TestNamesNumComponents(t *testing.T) {
	EnableDebugLogging(testing.Verbose())

	assertNumComps := func(n string, e int) { require.Equal(t, e, nameNumComponents(n), "name num components failed") }

	assertNumComps("", 0)
	assertNumComps(".", 0)
	assertNumComps("something", 1)
	assertNumComps("something.", 1)
	assertNumComps("something.local", 2)
	assertNumComps("something.local.", 2)
}

func TestNamesNormalization(t *testing.T) {
	EnableDebugLogging(testing.Verbose())

	assertNormalization := func(s, d, e string) { require.Equal(t, e, normalizeName(s, d), "name normalization failed") }

	assertNormalization("", "domain.local.", ".")
	assertNormalization(".", "domain.local.", ".")
	assertNormalization("something", "domain.local.", "something.domain.local.")
	assertNormalization("something.", "domain.local.", "something.domain.local.")
	assertNormalization("something.domain.local", "domain.local.", "something.domain.local.")
	assertNormalization("something.domain.local.", "domain.local.", "something.domain.local.")
}

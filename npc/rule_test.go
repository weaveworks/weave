package npc

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRegressionHandleNil3095(t *testing.T) {
	// Test for handling nil values
	// https://github.com/weaveworks/weave/issues/3095

	rs := newRuleSpecAllow(nil, nil, nil, nil)

	require.NotEqual(t, rs.key, "")

}

package npc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegressionHandleNil3095(t *testing.T) {
	// Test for handling nil values
	// https://github.com/rajch/weave/issues/3095

	rs := newRuleSpec(policyTypeIngress, nil, nil, nil, nil)

	require.NotEqual(t, rs.key, "")

}

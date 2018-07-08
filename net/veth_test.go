package net

import (
	"testing"
)

func TestIfShorten(t *testing.T) {

	shortIfName := getShortIfName("foobar0", 4)
	if shortIfName != "bar0" {
		t.Errorf("Sum was incorrect, got: %s, want: %s.", shortIfName, "bar0")
	}

	shortIfName = getShortIfName("foobar0", 3)
	if shortIfName != "ar0" {
		t.Errorf("Sum was incorrect, got: %s, want: %s.", shortIfName, "ar0")
	}

}

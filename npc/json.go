package npc

import (
	"encoding/json"
)

// Return JSON suitable for logging an API object.
func js(v interface{}) string {
	// Get the raw JSON
	a, _ := json.Marshal(v)
	// Convert this back into a tree of key-value maps
	var m map[string]interface{}
	if err := json.Unmarshal(a, &m); err != nil {
		// If that didn't work, just return the raw version
		return string(a)
	}
	// Trim some bulk, and potentially sensitive areas
	withMap(m["metadata"], func(status map[string]interface{}) {
		delete(status, "ownerReferences")
	})
	withMap(m["spec"], func(spec map[string]interface{}) {
		delete(spec, "tolerations")
		delete(spec, "volumes")
		rangeSlice(spec["containers"], func(container map[string]interface{}) {
			delete(container, "args")
			delete(container, "command")
			delete(container, "env")
			delete(container, "livenessProbe")
			delete(container, "resources")
			delete(container, "securityContext")
			delete(container, "volumeMounts")
		})
	})
	withMap(m["status"], func(status map[string]interface{}) {
		delete(status, "containerStatuses")
	})
	// Now marshall what's left to JSON
	a, _ = json.Marshal(m)
	return string(a)
}

// Helper function: operate on a map node from a tree of key-value maps
func withMap(m interface{}, f func(map[string]interface{})) {
	if v, ok := m.(map[string]interface{}); ok {
		f(v)
	}
}

// Helper function: operate on all nodes under i which is a slice in a
// tree of key-value maps
func rangeSlice(i interface{}, f func(map[string]interface{})) {
	if s, ok := i.([]interface{}); ok {
		for _, v := range s {
			if m, ok := v.(map[string]interface{}); ok {
				f(m)
			}
		}
	}
}

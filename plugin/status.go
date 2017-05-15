package plugin

type Status struct {
	DriverName     string
	MeshDriverName string `json:"MeshDriverName,omitempty"`
	Version        int
}

func NewStatus(address, meshAddress string, isPluginV2 bool) *Status {
	status := &Status{
		DriverName:     pluginNameFromAddress(address),
		MeshDriverName: pluginNameFromAddress(meshAddress),
		Version:        1,
	}
	if isPluginV2 {
		status.Version = 2
	}
	return status
}

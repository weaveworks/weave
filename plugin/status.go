package plugin

type Status struct {
	DriverName     string
	MeshDriverName string `json:"MeshDriverName,omitempty"`
	Version        int
}

func (plugin *Plugin) NewStatus() *Status {
	if plugin == nil {
		return nil
	}
	status := &Status{
		DriverName:     pluginNameFromAddress(plugin.Socket),
		MeshDriverName: pluginNameFromAddress(plugin.MeshSocket),
		Version:        1,
	}
	if plugin.EnableV2 {
		status.Version = 2
	}
	return status
}

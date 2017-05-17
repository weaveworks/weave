package plugin

type Status struct {
	Config         Config
	DriverName     string
	MeshDriverName string `json:"MeshDriverName,omitempty"`
}

func (plugin *Plugin) NewStatus() *Status {
	if plugin == nil {
		return nil
	}
	status := &Status{
		DriverName:     pluginNameFromAddress(plugin.Socket),
		MeshDriverName: pluginNameFromAddress(plugin.MeshSocket),
		Config:         plugin.Config,
	}
	return status
}

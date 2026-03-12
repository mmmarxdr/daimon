package tool

import "microagent/internal/config"

func BuildRegistry(cfg config.ToolsConfig) map[string]Tool {
	registry := make(map[string]Tool)

	if cfg.Shell.Enabled {
		st := NewShellTool(cfg.Shell)
		registry[st.Name()] = st
	}

	if cfg.File.Enabled {
		rt := NewReadFileTool(cfg.File)
		wt := NewWriteFileTool(cfg.File)
		lt := NewListFilesTool(cfg.File)
		registry[rt.Name()] = rt
		registry[wt.Name()] = wt
		registry[lt.Name()] = lt
	}

	if cfg.HTTP.Enabled {
		ht := NewHTTPFetchTool(cfg.HTTP)
		registry[ht.Name()] = ht
	}

	return registry
}

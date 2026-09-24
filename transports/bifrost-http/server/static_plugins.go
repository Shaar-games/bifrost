package server

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/transports/bifrost-http/lib"
)

// StaticPluginFactory constructs a plugin compiled into the HTTP transport.
// Static plugins provide the same configuration contract as dynamic plugins,
// but do not depend on Go's platform-limited buildmode=plugin support.
type StaticPluginFactory func(context.Context, any, *lib.Config) (schemas.BasePlugin, error)

var staticPluginRegistry = struct {
	sync.RWMutex
	factories map[string]StaticPluginFactory
}{factories: make(map[string]StaticPluginFactory)}

// RegisterStaticPlugin makes a statically linked plugin available by name.
// Registration must happen before BifrostHTTPServer.Bootstrap is called.
func RegisterStaticPlugin(name string, factory StaticPluginFactory) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("static plugin name cannot be empty")
	}
	if factory == nil {
		return fmt.Errorf("static plugin %q has a nil factory", name)
	}

	staticPluginRegistry.Lock()
	defer staticPluginRegistry.Unlock()
	if _, exists := staticPluginRegistry.factories[name]; exists {
		return fmt.Errorf("static plugin %q is already registered", name)
	}
	staticPluginRegistry.factories[name] = factory
	return nil
}

func instantiateStaticPlugin(ctx context.Context, name string, pluginConfig any, bifrostConfig *lib.Config) (schemas.BasePlugin, bool, error) {
	staticPluginRegistry.RLock()
	factory, ok := staticPluginRegistry.factories[name]
	staticPluginRegistry.RUnlock()
	if !ok {
		return nil, false, nil
	}
	plugin, err := factory(ctx, pluginConfig, bifrostConfig)
	if err != nil {
		return nil, true, fmt.Errorf("failed to initialize static plugin %q: %w", name, err)
	}
	if plugin == nil {
		return nil, true, fmt.Errorf("static plugin %q returned nil", name)
	}
	if plugin.GetName() != name {
		return nil, true, fmt.Errorf("static plugin %q returned plugin named %q", name, plugin.GetName())
	}
	return plugin, true, nil
}

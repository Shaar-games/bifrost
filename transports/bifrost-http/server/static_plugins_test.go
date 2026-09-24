package server

import (
	"context"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/transports/bifrost-http/lib"
)

type staticTestPlugin struct{ name string }

func (p *staticTestPlugin) GetName() string { return p.name }
func (*staticTestPlugin) Cleanup() error    { return nil }

func TestRegisterAndInstantiateStaticPlugin(t *testing.T) {
	name := "static-test-plugin"
	var received any
	if err := RegisterStaticPlugin(name, func(_ context.Context, config any, _ *lib.Config) (schemas.BasePlugin, error) {
		received = config
		return &staticTestPlugin{name: name}, nil
	}); err != nil {
		t.Fatal(err)
	}

	plugin, found, err := instantiateStaticPlugin(context.Background(), name, map[string]any{"enabled": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !found || plugin.GetName() != name {
		t.Fatalf("found=%v plugin=%#v", found, plugin)
	}
	if received == nil {
		t.Fatal("factory did not receive plugin config")
	}
}

func TestRegisterStaticPluginRejectsInvalidRegistration(t *testing.T) {
	if err := RegisterStaticPlugin("", func(context.Context, any, *lib.Config) (schemas.BasePlugin, error) {
		return &staticTestPlugin{name: "unused"}, nil
	}); err == nil {
		t.Fatal("expected empty name to fail")
	}
	if err := RegisterStaticPlugin("static-nil-factory", nil); err == nil {
		t.Fatal("expected nil factory to fail")
	}
}

func TestInstantiateStaticPluginValidatesReturnedName(t *testing.T) {
	name := "static-wrong-name"
	if err := RegisterStaticPlugin(name, func(context.Context, any, *lib.Config) (schemas.BasePlugin, error) {
		return &staticTestPlugin{name: "different"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, found, err := instantiateStaticPlugin(context.Background(), name, nil, nil)
	if !found || err == nil || !strings.Contains(err.Error(), "returned plugin named") {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

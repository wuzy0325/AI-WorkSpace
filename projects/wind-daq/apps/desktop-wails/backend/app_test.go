package backend

import (
	"testing"

	"wind-daq/services/api-go/pkg/appcontext"
)

func TestConfigLoadNilAppReturnsError(t *testing.T) {
	var app *App

	if _, err := app.ConfigLoad("storage-settings"); err == nil {
		t.Fatal("expected nil app ConfigLoad to return an error")
	}
}

func TestConfigSaveNilAppReturnsError(t *testing.T) {
	var app *App

	res := app.ConfigSave("storage-settings", `{}`)
	if res.Success || res.Error == "" {
		t.Fatalf("expected nil app ConfigSave to fail, got %#v", res)
	}
}

func TestConfigLoadReturnsDecodedConfig(t *testing.T) {
	ctx, err := appcontext.NewAppContext(t.TempDir())
	if err != nil {
		t.Fatalf("create app context: %v", err)
	}
	app := &App{appContext: ctx}

	res := app.ConfigSave("storage-settings", `{"baseDirectory":"data/recordings","filePrefix":"run"}`)
	if !res.Success {
		t.Fatalf("save config failed: %#v", res)
	}

	loaded, err := app.ConfigLoad("storage-settings")
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	data, ok := loaded["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected decoded config object, got %#v", loaded["data"])
	}
	if data["filePrefix"] != "run" {
		t.Fatalf("expected filePrefix run, got %#v", data["filePrefix"])
	}
}

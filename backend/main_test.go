package main

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
)

func TestInvokeRejectsInvalidArguments(t *testing.T) {
	app := &App{}
	for _, test := range []struct {
		name string
		args []json.RawMessage
	}{
		{"MissingMethod", nil},
		{"SetLanguage", nil},
		{"SetLanguage", []json.RawMessage{json.RawMessage(`123`)}},
	} {
		if _, err := invoke(app, test.name, test.args); err == nil {
			t.Fatalf("accepted invalid request: %s", test.name)
		}
	}
}

func TestDesktopAllowlistResolvesMethods(t *testing.T) {
	methods, err := allowedMethods()
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) == 0 {
		t.Fatal("allowlist is empty")
	}
	app := &App{}
	for name := range methods {
		if name == "startup" || name == "shutdown" {
			t.Fatalf("internal method exposed: %s", name)
		}
		if !reflect.ValueOf(app).MethodByName(name).IsValid() {
			t.Fatalf("missing method: %s", name)
		}
	}
}

// Methods that accept local filesystem paths must not be callable by the renderer.
func TestRendererCannotPassLocalPaths(t *testing.T) {
	var lists struct {
		Renderer []string `json:"renderer"`
	}
	if err := json.Unmarshal(methodList, &lists); err != nil {
		t.Fatal(err)
	}
	for _, name := range lists.Renderer {
		if name == "UploadPaths" || name == "SetCloseToTray" {
			t.Fatalf("host-only method exposed to the renderer: %s", name)
		}
	}
}

// The About dialog shows constants that must stay equal to the package
// manifest, and the packaged application must ship the method allowlist.
func TestMetadataMatchesPackageManifest(t *testing.T) {
	data, err := os.ReadFile("../package.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Build struct {
			ProductName string   `json:"productName"`
			Copyright   string   `json:"copyright"`
			Files       []string `json:"files"`
		} `json:"build"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Build.ProductName != appName || manifest.Build.Copyright != appCopyright {
		t.Fatalf("manifest has %q / %q", manifest.Build.ProductName, manifest.Build.Copyright)
	}
	if !slices.Contains(manifest.Build.Files, "backend/api-methods.json") {
		t.Fatalf("allowlist is not packaged: %v", manifest.Build.Files)
	}
}

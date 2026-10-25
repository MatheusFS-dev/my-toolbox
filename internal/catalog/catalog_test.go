package catalog

import (
	"os"
	"strings"
	"testing"
)

func TestCatalogRejectsPresentationOnlyField(t *testing.T) {
	source, err := os.ReadFile("../../assets/payload/commands.json")
	if err != nil {
		t.Fatal(err)
	}
	modified := strings.Replace(string(source), `"name":`, `"PresentationEnvironment":"windows","name":`, 1)
	if modified == string(source) {
		t.Fatal("catalog fixture has no command name")
	}
	if _, err := LoadCatalog(strings.NewReader(modified)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadCatalog() error = %v, want unknown field", err)
	}
}

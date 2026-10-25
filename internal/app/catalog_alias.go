package app

import (
	"github.com/MatheusFS-dev/my-toolbox/internal/catalog"
	"io"
)

type Command = catalog.Command
type Catalog = catalog.Catalog
type Capability = catalog.Capability

var supportedPlatforms = catalog.SupportedPlatforms

func LoadCatalog(reader io.Reader) (Catalog, error) { return catalog.LoadCatalog(reader) }
func LoadCatalogFile(path string) (Catalog, error)  { return catalog.LoadCatalogFile(path) }
func ResolveRequirements(command Command, environment string) ([]Capability, error) {
	return catalog.ResolveRequirements(command, environment)
}

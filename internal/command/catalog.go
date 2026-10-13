package command

import (
	_ "embed"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"fleetmgr/internal/agentproto"
)

type yamlFile struct {
	Commands []agentproto.CatalogItem `yaml:"commands"`
}

type Catalog struct {
	items map[string]agentproto.CatalogItem
	list  []agentproto.CatalogItem
}

//go:embed commands.yaml
var defaultCatalog []byte

// LoadDefault returns the catalog compiled into the binary.
func LoadDefault() (*Catalog, error) { return parse(defaultCatalog) }

// Load reads a catalog from path; an empty path means the embedded default.
func Load(path string) (*Catalog, error) {
	if path == "" {
		return LoadDefault()
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parse(b)
}

// Allows reports whether argv exactly matches the catalog entry for id.
func (c *Catalog) Allows(id string, argv []string) bool {
	item, ok := c.items[id]
	if !ok || len(item.Argv) != len(argv) {
		return false
	}
	for i := range argv {
		if argv[i] != item.Argv[i] {
			return false
		}
	}
	return true
}

func parse(b []byte) (*Catalog, error) {
	var f yamlFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	c := &Catalog{items: map[string]agentproto.CatalogItem{}}
	for _, item := range f.Commands {
		if item.ID == "" || len(item.Argv) == 0 {
			return nil, fmt.Errorf("invalid catalog entry %q", item.ID)
		}
		if item.TimeoutSeconds <= 0 {
			item.TimeoutSeconds = 5
		}
		c.items[item.ID] = item
		c.list = append(c.list, item)
	}
	return c, nil
}

func (c *Catalog) Get(id string) (agentproto.CatalogItem, bool) {
	item, ok := c.items[id]
	return item, ok
}

func (c *Catalog) List() []agentproto.CatalogItem {
	return c.list
}

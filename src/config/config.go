package config

import (
	"fmt"
	"os"
	"renovate-operator/assert"
)

type ConfigItemDescription struct {
	Optional bool
	Default  string
	Key      string
	Validate func(value string) error
}

type ConfigItem struct {
	Key   string
	Value string
}
type configModule struct {
	settings map[string]ConfigItem
}

var staticConfigModule *configModule

func InitializeConfigModule(configs []ConfigItemDescription) error {
	staticConfigModule = &configModule{
		settings: make(map[string]ConfigItem, len(configs)),
	}

	for i := range configs {
		configDeclaration := configs[i]

		envVar := os.Getenv(configDeclaration.Key)

		if envVar == "" && !configDeclaration.Optional {
			return fmt.Errorf("option %s is not set", configDeclaration.Key)
		}
		value := configDeclaration.Default
		if envVar != "" {
			value = envVar
		}

		if configDeclaration.Validate != nil {
			err := configDeclaration.Validate(value)
			if err != nil {
				return err
			}
		}

		staticConfigModule.settings[configDeclaration.Key] = ConfigItem{
			Key:   configDeclaration.Key,
			Value: value,
		}

	}
	return nil
}

func GetValue(key string) string {
	assert.Assert(staticConfigModule != nil, "static config module has never been initialized")
	item := staticConfigModule.settings[key]

	return item.Value
}

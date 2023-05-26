package metrics

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

// TODO: finalize this once the init script is final
//go:embed init.gradle.gotemplate
var initTemplate string

type templateInventory struct {
	Version   string
	Endpoint  string
	Port      int
	AuthToken string
}

func renderTemplate(inventory templateInventory) (string, error) {
	if inventory.Version == "" {
		return "", fmt.Errorf("version cannot be empty")
	}
	if inventory.Endpoint == "" {
		return "", fmt.Errorf("endpoint cannot be empty")
	}

	if inventory.AuthToken == "" {
		return "", fmt.Errorf("auth token cannot be empty")
	}

	if inventory.Port == 0 {
		return "", fmt.Errorf("invalid port number: %d", inventory.Port)
	}

	tmpl, err := template.New("init.gradle").Parse(initTemplate)
	if err != nil {
		return "", fmt.Errorf("invalid template: %w", err)
	}

	resultBuffer := bytes.Buffer{}
	if err = tmpl.Execute(&resultBuffer, inventory); err != nil {
		return "", err
	}
	return resultBuffer.String(), nil
}

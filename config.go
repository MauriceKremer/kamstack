package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// The bundled Pi defaults, so setup needs no network (DST-15). They are the
// same files the release archives ship: kam.yml already points at
// /dev/ttyUSB0 instead of the maintainer's macOS serial device.
//
//go:embed defaults/kam.yml defaults/kammon.yml
var defaultConfigs embed.FS

func defaultKamConfig() ([]byte, error) {
	return defaultConfigs.ReadFile("defaults/kam.yml")
}

func defaultKammonConfig() ([]byte, error) {
	return defaultConfigs.ReadFile("defaults/kammon.yml")
}

// renderKamConfig writes the KAM config with the install's node name, database
// path and serial port, keeping the rest of the bundled defaults untouched.
func renderKamConfig(nodeName, dbPath, port string) ([]byte, error) {
	raw, err := defaultKamConfig()
	if err != nil {
		return nil, err
	}
	doc, err := decodeConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("bundled kam.yml: %w", err)
	}
	root := documentRoot(doc)
	setKey(root, "node_name", nodeName)
	setKey(root, "db_path", dbPath)
	radio, err := childMapping(root, "radio")
	if err != nil {
		return nil, fmt.Errorf("bundled kam.yml: %w", err)
	}
	setKey(radio, "port", port)
	return yaml.Marshal(doc)
}

// renderKammonConfig writes the KAMmon config with generated passwords and the
// KAM URL setup guarantees.
func renderKammonConfig(adminPassword, viewerPassword, kamURL string) ([]byte, error) {
	raw, err := defaultKammonConfig()
	if err != nil {
		return nil, err
	}
	doc, err := decodeConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("bundled kammon.yml: %w", err)
	}
	root := documentRoot(doc)
	setKey(root, "kam_monitor_url", kamURL)
	users, err := childSequence(root, "users")
	if err != nil {
		return nil, fmt.Errorf("bundled kammon.yml: %w", err)
	}
	for _, user := range users.Content {
		switch getKey(user, "name") {
		case "admin":
			setKey(user, "password", adminPassword)
		case "viewer":
			setKey(user, "password", viewerPassword)
		}
	}
	return yaml.Marshal(doc)
}

func decodeConfig(raw []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		return doc.Content[0]
	}
	return doc
}

func childMapping(mapping *yaml.Node, key string) (*yaml.Node, error) {
	if value := findValue(mapping, key); value != nil {
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s is not a map", key)
		}
		return value, nil
	}
	return nil, fmt.Errorf("%s is missing", key)
}

func childSequence(mapping *yaml.Node, key string) (*yaml.Node, error) {
	if value := findValue(mapping, key); value != nil {
		if value.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("%s is not a list", key)
		}
		return value, nil
	}
	return nil, fmt.Errorf("%s is missing", key)
}

func findValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setKey(mapping *yaml.Node, key, value string) {
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = scalar
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, scalar)
}

func getKey(mapping *yaml.Node, key string) string {
	if value := findValue(mapping, key); value != nil {
		return value.Value
	}
	return ""
}

// readKamConfigNode loads the installed kam.yml for a small edit.
func readConfigNode(path string) (*yaml.Node, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := decodeConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

// writeConfig writes atomically and leaves the file group-writable: KAM
// rewrites kam.yml at runtime and runs as the kamstack user (DST-16, DST-17).
func writeConfig(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o660); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func updateKamPort(path, port string) error {
	doc, err := readConfigNode(path)
	if err != nil {
		return err
	}
	radio, err := childMapping(documentRoot(doc), "radio")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	setKey(radio, "port", port)
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return writeConfig(path, raw)
}

// kamRadioPort reports the installed serial path, or "" when it cannot be read.
func kamRadioPort(path string) string {
	doc, err := readConfigNode(path)
	if err != nil {
		return ""
	}
	radio, err := childMapping(documentRoot(doc), "radio")
	if err != nil {
		return ""
	}
	return getKey(radio, "port")
}

func configListenURL(path, section, fallback string) string {
	doc, err := readConfigNode(path)
	if err != nil {
		return fallback
	}
	listen := getKey(documentRoot(doc), "listen")
	if section != "" {
		if parent, err := childMapping(documentRoot(doc), section); err == nil {
			listen = getKey(parent, "listen")
		}
	}
	if listen == "" {
		return fallback
	}
	if strings.HasPrefix(listen, ":") {
		return "http://127.0.0.1" + listen
	}
	return "http://" + listen
}

func kamAPIBase(path string) string {
	return configListenURL(path, "monitor", kamAPIURL)
}

func kammonWebBase(path string) string {
	return configListenURL(path, "", kammonWebURL)
}

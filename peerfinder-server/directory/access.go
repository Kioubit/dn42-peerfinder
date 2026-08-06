package directory

import (
	"errors"
	"fmt"
	"os"
	"peerfinder-db/directory/directoryTypes"
	"time"

	"github.com/goccy/go-yaml"
)

var errYAMLFileNotFound = errors.New("yaml file not found")

func ReadYAMLFile(dir, name string) (*directoryTypes.YamlNetwork, time.Time, error) {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to open root: %w", err)
	}
	defer func() {
		_ = r.Close()
	}()

	var c directoryTypes.YamlNetwork

	stat, err := r.Stat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, time.Time{}, fmt.Errorf("%w: %s", errYAMLFileNotFound, name)
		}
		return nil, time.Time{}, fmt.Errorf("failed to stat file: %w", err)
	}

	modTime := stat.ModTime()

	data, err := r.ReadFile(name)
	if err != nil {
		return nil, modTime, fmt.Errorf("failed to read file: %w", err)
	}

	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, modTime, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	return &c, modTime, nil
}

func writeYAMLFile(dir, name string, entry directoryTypes.YamlNetwork) error {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("failed to open root: %w", err)
	}
	defer func() {
		_ = r.Close()
	}()

	output, err := yaml.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	if err := r.WriteFile(name, output, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

func deleteYAMLFile(dir, name string) error {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("failed to open root: %w", err)
	}
	defer func() {
		_ = r.Close()
	}()
	err = r.Remove(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

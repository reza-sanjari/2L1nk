package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed changelog.json
var changelogData []byte

type ChangelogEntry struct {
	Version string   `json:"version"`
	Date    string   `json:"date"`
	Notes   []string `json:"notes"`
}

type InfoResponse struct {
	Version   string           `json:"version"`
	Changelog []ChangelogEntry `json:"changelog"`
}

func GetInfo() (*InfoResponse, error) {
	var changelog []ChangelogEntry
	if err := json.Unmarshal(changelogData, &changelog); err != nil {
		return nil, fmt.Errorf("parse changelog.json: %w", err)
	}

	version := "dev"
	if len(changelog) > 0 {
		version = changelog[0].Version
	}

	return &InfoResponse{
		Version:   version,
		Changelog: changelog,
	}, nil
}

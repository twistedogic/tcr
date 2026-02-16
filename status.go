package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Change struct {
	Name         string    `json:"name"`
	LastModified time.Time `json:"lastModified"`
	Status       string    `json:"status"`
}

type ListChange struct {
	Changes []Change `json:"changes"`
}

func listChanges(ctx context.Context, path string) ([]string, error) {
	var list ListChange
	if err := executeJSON(ctx, &list, path, "openspec", "list", "--json"); err != nil {
		return nil, fmt.Errorf("unmarshal openspec list: %w", err)
	}
	changes := make([]string, 0, len(list.Changes))
	for _, c := range list.Changes {
		changes = append(changes, c.Name)
	}
	return changes, nil
}

type Artifact struct {
	Id          string   `json:"id"`
	Status      string   `json:"status"`
	MissingDeps []string `json:"missingDeps,omitempty"`
}

type Status struct {
	ChangeName    string     `json:"changeName"`
	IsComplete    bool       `json:"isComplete"`
	ApplyRequires []string   `json:"applyRequires,omitempty"`
	Artifacts     []Artifact `json:"artifacts,omitempty"`
}

func (s *Status) String() string {
	switch {
	case s == nil:
		return "No openspec setup"
	case s.ChangeName == "":
		return "Ready For Change"
	case s.IsComplete:
		return "Ready For Review"
	case len(s.ApplyRequires) == 0:
		return "Ready For Apply"
	default:
		return fmt.Sprintf("Pending – %s", strings.Join(s.ApplyRequires, ", "))
	}
}

func showChange(ctx context.Context, path, changeName string) (Status, error) {
	var status Status
	if err := executeJSON(ctx, &status, path, "openspec", "status", "--change", changeName, "--json"); err != nil {
		return status, fmt.Errorf("unmarshal openspec status: %w", err)
	}
	return status, nil
}

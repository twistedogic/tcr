package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type Change struct {
	Name           string    `json:"name"`
	CompletedTasks int       `json:"completedTasks"`
	TotalTasks     int       `json:"totalTasks"`
	LastModified   time.Time `json:"lastModified"`
	Status         string    `json:"status"`
}
type ListChange struct {
	Changes []Change `json:"changes"`
}

func listChanges(ctx context.Context, path string) ([]string, error) {
	var list ListChange
	b, err := execute(ctx, path, "openspec", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, b)
	}
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	changes := make([]string, 0, len(list.Changes))
	for _, c := range list.Changes {
		changes = append(changes, c.Name)
	}
	return changes, nil
}

type Artifact struct {
	Id          string
	OutputPath  string
	Status      string
	MissingDeps []string
}

type Status struct {
	ChangeName    string
	IsComplete    bool
	ApplyRequires []string
	Artifacts     []Artifact
}

func (s Status) ReadyForDevelop() bool {
	return len(s.ApplyRequires) == 0
}

func showChange(ctx context.Context, path, changeName string) (Status, error) {
	var status Status
	b, err := execute(ctx, path, "openspec", "status", "--change", changeName, "--json")
	if err != nil {
		return status, err
	}
	err = json.Unmarshal(b, &status)
	return status, err
}

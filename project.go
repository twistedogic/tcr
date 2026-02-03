package main

import "context"

type Project struct {
	path   string
	Status Status
}

func (p *Project) Refresh(ctx context.Context) error {
	changes, err := listChanges(ctx, p.path)
	if err != nil {
		return err
	}
	for _, c := range changes {
		status, err := showChange(ctx, p.path, c)
		if err != nil {
			return err
		}
		if !status.IsComplete {
			p.Status = status
			return nil
		}
	}
	return nil
}

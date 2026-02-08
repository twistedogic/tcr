package main

import "context"

func review(ctx context.Context, path string) ([]byte, error) {
	return execute(ctx, path, "tuicr", "--stdout")
}

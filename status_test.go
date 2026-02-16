package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Status(t *testing.T) {
	b, err := os.ReadFile("testdata/status.json")
	require.NoError(t, err)
	b, err = cleanOutputJSON(b)
	require.NoError(t, err)
	var s Status
	require.NoError(t, json.Unmarshal(b, &s))
}

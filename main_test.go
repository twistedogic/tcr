package main

import (
	"bytes"
	"os"
	"testing"
)

func Test_tcr(t *testing.T) {
	review, err := parseJSON("testdata/projector_388e9be_20260127_152442.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/prompt.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, []byte(generateMarkdown(review))) {
		t.Fail()
	}
}

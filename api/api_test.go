package api

import (
	"bytes"
	"testing"
)

func TestSpecIsEmbedded(t *testing.T) {
	t.Parallel()
	if !bytes.HasPrefix(Spec, []byte("openapi: 3.1.0\n")) {
		t.Errorf("Spec starts %q, want the OpenAPI 3.1 contract", Spec[:min(len(Spec), 20)])
	}
}

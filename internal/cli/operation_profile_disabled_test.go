//go:build !searchprofile

package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestBenchmarkOperationProfileRequiresSearchprofileBuildTag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run([]string{"benchmark-scenarios", "--operation-profile"}, &stdout, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "requires a binary built with -tags searchprofile") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

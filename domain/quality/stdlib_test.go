package quality

import (
	"os/exec"
	"strings"
	"testing"
)

// TestStdlibCoverage fails if go list std reports an importable package with
// no classification entry, so newly added stdlib packages force a decision
// instead of silently falling through to Unknown.
func TestStdlibCoverage(t *testing.T) {
	output, err := exec.Command("go", "list", "std").Output()
	if err != nil {
		t.Fatalf("go list std: %v", err)
	}
	prefixes := DefaultConfig().Prefixes
	var missing []string
	for _, pkg := range strings.Fields(string(output)) {
		if !importable(pkg) {
			continue
		}
		if !coveredByPrefixes(pkg, prefixes) {
			missing = append(missing, pkg)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("stdlib packages without classification:\n  %s", strings.Join(missing, "\n  "))
	}
}

// importable reports whether user code outside the standard library can
// import pkg. Internal and vendored packages cannot.
func importable(pkg string) bool {
	return !strings.HasPrefix(pkg, "internal/") &&
		!strings.Contains(pkg, "/internal/") &&
		!strings.HasSuffix(pkg, "/internal") &&
		!strings.Contains(pkg, "vendor/")
}

func coveredByPrefixes(pkg string, prefixes []Prefix) bool {
	blanket := pkg + "."
	for _, prefix := range prefixes {
		if prefix.Path == blanket || strings.HasPrefix(prefix.Path, blanket) {
			return true
		}
	}
	return false
}

func TestImportable(t *testing.T) {
	for pkg, want := range map[string]bool{
		"strings":                 true,
		"net/http":                true,
		"internal/abi":            false,
		"crypto/internal/boring":  false,
		"database/sql/internal":   false,
		"vendor/golang.org/x/sys": false,
	} {
		if importable(pkg) != want {
			t.Fatalf("importable(%q) = %v", pkg, !want)
		}
	}
}

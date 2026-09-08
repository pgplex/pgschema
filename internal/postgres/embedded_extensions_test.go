package postgres

import (
	"errors"
	"testing"
)

// TestInstallUntilFixpoint simulates extension prerequisites that sort after
// their dependents (hstore_plperl requires plperl), which a single alphabetical
// pass cannot install (issue #584 review).
func TestInstallUntilFixpoint(t *testing.T) {
	requires := map[string][]string{
		"hstore_plperl": {"hstore", "plperl"},
		"bool_plperl":   {"plperl"},
		"postgis":       {"libpostgis"}, // never satisfiable: not bundled
	}
	installed := map[string]bool{}
	var calls []string

	install := func(name string) error {
		calls = append(calls, name)
		for _, dep := range requires[name] {
			if !installed[dep] {
				return errors.New("required extension " + dep + " is not installed")
			}
		}
		installed[name] = true
		return nil
	}

	// Alphabetical, as installTargetExtensions feeds it.
	names := []string{"bool_plperl", "hstore", "hstore_plperl", "plperl", "postgis"}
	unavailable := installUntilFixpoint(names, install)

	for _, name := range []string{"bool_plperl", "hstore", "hstore_plperl", "plperl"} {
		if !installed[name] {
			t.Errorf("%s should have been installed once its prerequisites were", name)
		}
		if _, ok := unavailable[name]; ok {
			t.Errorf("%s should not be reported unavailable", name)
		}
	}
	if err, ok := unavailable["postgis"]; !ok {
		t.Errorf("postgis should be reported unavailable")
	} else if err == nil {
		t.Errorf("unavailable entry should carry the last error")
	}

	// Pass 1: 5 calls (2 fail on plperl, postgis fails). Pass 2: 3 retries (postgis
	// fails). Pass 3: postgis only, still fails, no progress -> stop.
	if len(calls) != 9 {
		t.Errorf("expected 9 install calls (5 + 3 + 1), got %d: %v", len(calls), calls)
	}
}

func TestInstallUntilFixpoint_Empty(t *testing.T) {
	called := false
	unavailable := installUntilFixpoint(nil, func(string) error { called = true; return nil })
	if called || len(unavailable) != 0 {
		t.Errorf("no names should mean no calls and nothing unavailable")
	}
}

package diff

import "testing"

func TestDisplayKey(t *testing.T) {
	tests := map[string]string{
		"US":                            "US",
		"a" + keySeparator + "b":        "a,b",
		"a,b" + keySeparator + "c":      `"a,b",c`,
		"a" + keySeparator + "b,c":      `a,"b,c"`,
		`say "hi"` + keySeparator + "x": `"say ""hi""",x`,
		"" + keySeparator + "":          ",",
	}
	for key, want := range tests {
		if got := displayKey(key); got != want {
			t.Errorf("displayKey(%q) = %q, want %q", key, got, want)
		}
	}
}

package gitlab

import "testing"

func TestFileExt(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{path: "Main.GO", want: "go"},
		{path: "main.go", want: "go"},
		{path: "a/b/service.test.ts", want: "ts"},
		{path: "Makefile", want: ""},
		{path: ".gitignore", want: ""},
		{path: "archive.tar.gz", want: "gz"},
		{path: "noext.", want: ""},
		{path: "", want: ""},
		{path: "src/index.js", want: "js"},
		{path: "/absolute/path/file.go", want: "go"},
		{path: "UPPERCASE.SH", want: "sh"},
	}
	for _, c := range cases {
		got := fileExt(c.path)
		if got != c.want {
			t.Errorf("fileExt(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

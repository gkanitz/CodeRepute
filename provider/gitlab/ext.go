package gitlab

import "strings"

// fileExt reduces a file path to its canonical extension form:
// substring after the final dot of the basename, lowercased, no
// leading dot; "" when the basename has no dot or its only dot
// starts the name or ends the name.
//
// This is the single point of extension reduction for the GitLab
// adapter. It is tested directly and gives a type-level guarantee
// that full paths are never copied into the provided FileStat.
func fileExt(path string) string {
	// Find the last / to get the basename.
	idx := strings.LastIndexByte(path, '/')
	base := path
	if idx >= 0 {
		base = path[idx+1:]
	}

	// Find the last dot in the basename.
	dot := strings.LastIndexByte(base, '.')
	if dot <= 0 || dot == len(base)-1 {
		// No dot, dot at position 0 (.gitignore), or dot at the
		// end of the name (noext.) — all produce "".
		return ""
	}

	return strings.ToLower(base[dot+1:])
}

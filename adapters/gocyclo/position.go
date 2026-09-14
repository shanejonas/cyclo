package gocyclo

import "go/token"

// The TUI displays the source on disk. Preserve token offsets and physical line
// boundaries, but omit //line mappings to other files and logical line numbers.
func physicalFileSet(source *token.File) *token.FileSet {
	set := token.NewFileSet()
	file := set.AddFile(source.Name(), source.Base(), source.Size())
	file.SetLines(source.Lines())
	return set
}

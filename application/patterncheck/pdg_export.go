package patterncheck

import (
	"context"
	"fmt"
	"io"

	"github.com/shanejonas/cyclo/adapters/gopatterns"
	"github.com/shanejonas/cyclo/adapters/pdgjson"
	"github.com/shanejonas/cyclo/domain/pdg"
)

// exportPDGs bypasses mining and its caches. The array contains one compat
// document per function; documents are expanded and encoded one at a time.
func exportPDGs(ctx context.Context, opts options, output io.Writer) error {
	ext, err := gopatterns.Extract(ctx, "", opts.paths)
	if err != nil {
		return err
	}
	if err := validateExport(ext.Funcs); err != nil {
		return err
	}
	return writePDGArray(ctx, output, ext.Funcs)
}

// Reject invalid native input before writing any export bytes.
func validateExport(funcs []gopatterns.FuncPdg) error {
	for i := range funcs {
		if err := pdg.Validate(&funcs[i].Pdg); err != nil {
			return fmt.Errorf("export %s: %w", funcs[i].Name, err)
		}
	}
	return nil
}

func writePDGArray(ctx context.Context, output io.Writer, funcs []gopatterns.FuncPdg) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := io.WriteString(output, "[\n"); err != nil {
		return err
	}
	for i := range funcs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := exportFunction(output, &funcs[i], i); err != nil {
			return err
		}
	}
	_, err := io.WriteString(output, "]\n")
	return err
}
func exportFunction(output io.Writer, f *gopatterns.FuncPdg, index int) error {
	if index != 0 {
		if _, err := io.WriteString(output, ",\n"); err != nil {
			return err
		}
	}
	if err := pdgjson.Write(output, &f.Pdg); err != nil {
		return fmt.Errorf("export %s: %w", f.Name, err)
	}
	return nil
}

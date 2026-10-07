package patterns

import (
	"go/types"
	"strings"
)

// TypeClass is the coarse class of a type for graph labels, porting rstyle's
// shape::class_of: primitives keep their name, stdlib ADTs keep their short
// name, every other named type is "_", so `[]User` and `[]Order` share a
// class while `[]string` stays distinct.
func TypeClass(t types.Type) string {
	switch t := t.(type) {
	case *types.Basic:
		return t.Name()
	case *types.Pointer:
		return "*" + TypeClass(t.Elem())
	case *types.Slice:
		return "[" + TypeClass(t.Elem()) + "]"
	case *types.Array:
		return "[" + TypeClass(t.Elem()) + "; _]"
	default:
		return typeClassComposite(t)
	}
}

func typeClassComposite(t types.Type) string {
	switch t := t.(type) {
	case *types.Map:
		return "map[" + TypeClass(t.Key()) + "]" + TypeClass(t.Elem())
	case *types.Chan:
		return "chan " + TypeClass(t.Elem())
	case *types.Signature:
		return "_"
	case *types.Interface:
		return "_"
	case *types.TypeParam:
		return "T" + t.Obj().Name()
	default:
		return typeClassNamed(t)
	}
}

func typeClassNamed(t types.Type) string {
	switch t := t.(type) {
	case *types.Named:
		return namedClass(t)
	case *types.Struct:
		return "_"
	case *types.Tuple:
		return tupleClass(t)
	default:
		return "_"
	}
}

func namedClass(t *types.Named) string {
	if t.Obj().Pkg() == nil && t.Obj().Name() == "error" {
		return "error"
	}
	if isStdlib(t.Obj().Pkg()) {
		return t.Obj().Name()
	}
	return "_"
}

func tupleClass(t *types.Tuple) string {
	parts := make([]string, t.Len())
	for i := range parts {
		parts[i] = TypeClass(t.At(i).Type())
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// isStdlib reports whether pkg is in the Go standard library. The heuristic
// is the conventional one: stdlib import paths contain no dot.
func isStdlib(pkg *types.Package) bool {
	return pkg != nil && !strings.Contains(pkg.Path(), ".")
}

// SigClass normalizes a function signature for Call labels, porting rstyle's
// shape::normalize: "fn(params) -> ret" with named types erased to "_". The
// receiver (if any) leads the parameter list, so methods and functions with
// otherwise identical shapes still differ. Variadic params render as "...T".
func SigClass(sig *types.Signature) string {
	params := []string{}
	if recv := sig.Recv(); recv != nil {
		params = append(params, TypeClass(recv.Type()))
	}
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		class := TypeClass(param.Type())
		if sig.Variadic() && i == sig.Params().Len()-1 {
			if slice, ok := param.Type().(*types.Slice); ok {
				class = "..." + TypeClass(slice.Elem())
			}
		}
		params = append(params, class)
	}
	return "fn(" + strings.Join(params, ", ") + ") -> " + resultClass(sig.Results())
}

func resultClass(results *types.Tuple) string {
	switch results.Len() {
	case 0:
		return "()"
	case 1:
		return TypeClass(results.At(0).Type())
	default:
		return TypeClass(results)
	}
}

// FuncID is the stable id of a called function for hole detection (never part
// of the WL label). Format matches goquality's resolvedName: package path
// plus name, with "Type.Method" for methods.
func FuncID(fn *types.Func) string {
	name := fn.Name()
	if sig, ok := fn.Type().(*types.Signature); ok {
		if recv := sig.Recv(); recv != nil {
			if named, ok := namedOf(recv.Type()); ok {
				name = named.Obj().Name() + "." + name
			}
		}
	}
	if fn.Pkg() == nil {
		return name
	}
	return fn.Pkg().Path() + "." + name
}

func namedOf(t types.Type) (*types.Named, bool) {
	t = types.Unalias(t)
	if pointer, ok := t.(*types.Pointer); ok {
		t = types.Unalias(pointer.Elem())
	}
	named, ok := t.(*types.Named)
	return named, ok
}

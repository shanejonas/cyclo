package goquality

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/shanejonas/cyclo/domain/quality"
)

type referenceBinding struct {
	values  []ast.Expr
	unknown bool
}

// Bindings retain all possible assignments. Provenance is conservative across
// branches and loops; it does not depend on the AST walk's execution order.
func (x *extractor) collectBindings(body *ast.BlockStmt) {
	ast.Inspect(body, func(node ast.Node) bool {
		x.addLiteralParameters(node)
		switch node := node.(type) {
		case *ast.AssignStmt:
			x.bindAssignments(node.Lhs, node.Rhs)
		case *ast.ValueSpec:
			places := make([]ast.Expr, len(node.Names))
			for index, name := range node.Names {
				places[index] = name
			}
			x.bindAssignments(places, node.Values)
		case *ast.RangeStmt:
			x.unknownBinding(node.Key)
			x.unknownBinding(node.Value)
		case *ast.UnaryExpr:
			x.addressBinding(node)
		}
		return true
	})
}

func (x *extractor) bindAssignments(places, values []ast.Expr) {
	for index, place := range places {
		id, ok := ast.Unparen(place).(*ast.Ident)
		if !ok {
			continue
		}
		object := x.pkg.TypesInfo.ObjectOf(id)
		if object == nil {
			continue
		}
		value := assignmentValue(values, index, len(places))
		binding := x.bindings[object]
		binding.values = append(binding.values, value)
		x.bindings[object] = binding
	}
}

func assignmentValue(values []ast.Expr, index, placeCount int) ast.Expr {
	if len(values) == 0 {
		return nil
	}
	if len(values) == placeCount {
		return values[index]
	}
	return values[0]
}

func (x *extractor) addressBinding(value *ast.UnaryExpr) {
	if value.Op != token.AND {
		return
	}
	// Taking a variable's address permits assignments outside direct bindings.
	x.unknownBinding(value.X)
}

func (x *extractor) unknownBinding(place ast.Expr) {
	id, ok := ast.Unparen(place).(*ast.Ident)
	if !ok {
		return
	}
	object := x.pkg.TypesInfo.ObjectOf(id)
	if object == nil {
		return
	}
	binding := x.bindings[object]
	binding.unknown = true
	x.bindings[object] = binding
}

func (x *extractor) mutate(place ast.Expr, pos token.Pos, storage bool) {
	if discardedPlace(place) {
		return
	}
	object, fields := x.placeRoot(place)
	if object == nil {
		x.temporaryMutation(place, pos, storage, fields)
		return
	}
	if packageVariable(object) {
		x.effect(quality.Global, resolvedVariable(object), pos)
		return
	}
	provenance := x.mutationProvenance(place, storage)
	x.fact.Mutations = append(x.fact.Mutations, quality.Mutation{
		Root: object.Name(), RootID: x.objectID(object), FieldPath: strings.Join(fields, "."), Line: x.line(pos), Provenance: provenance,
	})
}

func discardedPlace(place ast.Expr) bool {
	id, ok := ast.Unparen(place).(*ast.Ident)
	return ok && id.Name == "_"
}

func (x *extractor) placeRoot(place ast.Expr) (types.Object, []string) {
	switch place := place.(type) {
	case *ast.Ident:
		return x.pkg.TypesInfo.ObjectOf(place), nil
	case *ast.SelectorExpr:
		return x.selectorRoot(place)
	default:
		base := projectedBase(place)
		if base == nil {
			return nil, nil
		}
		return x.placeRoot(base)
	}
}

func (x *extractor) temporaryMutation(place ast.Expr, pos token.Pos, storage bool, fields []string) {
	provenance := x.mutationProvenance(place, storage)
	position := physicalPosition(x.pkg.Fset, pos)
	x.fact.Mutations = append(x.fact.Mutations, quality.Mutation{Root: "<temporary>", RootID: fmt.Sprintf("%d:%d", position.Line, position.Column), FieldPath: strings.Join(fields, "."), Line: position.Line, Provenance: provenance})
}

func (x *extractor) mutationProvenance(place ast.Expr, storage bool) quality.Provenance {
	if storage {
		return x.origin(place, map[types.Object]bool{})
	}
	return x.placeProvenance(place)
}

func projectedBase(place ast.Expr) ast.Expr {
	switch place := place.(type) {
	case *ast.IndexExpr:
		return place.X
	case *ast.SliceExpr:
		return place.X
	case *ast.StarExpr:
		return place.X
	case *ast.ParenExpr:
		return place.X
	default:
		return nil
	}
}

func (x *extractor) selectorRoot(place *ast.SelectorExpr) (types.Object, []string) {
	object := x.pkg.TypesInfo.ObjectOf(place.Sel)
	if object != nil && packageVariable(object) {
		return object, nil
	}
	root, fields := x.placeRoot(place.X)
	return root, append(fields, place.Sel.Name)
}

func underlying(typ types.Type) types.Type {
	if typ == nil {
		return nil
	}
	return typ.Underlying()
}

func (x *extractor) placeProvenance(place ast.Expr) quality.Provenance {
	switch place := place.(type) {
	case *ast.Ident:
		return quality.Local
	case *ast.ParenExpr:
		return x.placeProvenance(place.X)
	case *ast.StarExpr:
		return x.origin(place.X, map[types.Object]bool{})
	case *ast.SelectorExpr:
		return x.selectorProvenance(place)
	case *ast.IndexExpr:
		return x.indexProvenance(place)
	default:
		return quality.Unknown
	}
}

func (x *extractor) selectorProvenance(place *ast.SelectorExpr) quality.Provenance {
	if embeddedPointer(x.pkg.TypesInfo.Selections[place]) {
		return x.fieldOrigin(place)
	}
	if _, ok := underlying(x.pkg.TypesInfo.TypeOf(place.X)).(*types.Pointer); ok {
		return x.origin(place.X, map[types.Object]bool{})
	}
	return x.placeProvenance(place.X)
}

func embeddedPointer(selection *types.Selection) bool {
	if selection == nil {
		return false
	}
	typ := underlying(selection.Recv())
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = underlying(pointer.Elem())
	}
	// The compiler supplies struct fields for every step before the final field.
	for _, index := range selection.Index()[:len(selection.Index())-1] {
		typ = underlying(typ.(*types.Struct).Field(index).Type())
		if _, ok := typ.(*types.Pointer); ok {
			return true
		}
	}
	return false
}

func (x *extractor) indexProvenance(place *ast.IndexExpr) quality.Provenance {
	if valueArray(x.pkg.TypesInfo.TypeOf(place.X)) {
		return x.placeProvenance(place.X)
	}
	return x.origin(place.X, map[types.Object]bool{})
}

func (x *extractor) origin(value ast.Expr, seen map[types.Object]bool) quality.Provenance {
	switch value := value.(type) {
	case nil:
		return quality.Local // Zero-initialized reference contains no shared state.
	case *ast.Ident:
		return x.objectOrigin(x.pkg.TypesInfo.ObjectOf(value), seen)
	case *ast.ParenExpr:
		return x.origin(value.X, seen)
	case *ast.SliceExpr:
		return x.sliceOrigin(value, seen)
	default:
		return x.constructedOrigin(value)
	}
}

func (x *extractor) constructedOrigin(value ast.Expr) quality.Provenance {
	switch value := value.(type) {
	case *ast.UnaryExpr:
		return x.addressOrigin(value)
	case *ast.CompositeLit:
		return quality.Local
	case *ast.CallExpr:
		return x.allocationOrigin(value)
	case *ast.SelectorExpr:
		return x.fieldOrigin(value)
	default:
		return quality.Unknown
	}
}

func (x *extractor) addressOrigin(value *ast.UnaryExpr) quality.Provenance {
	if value.Op == token.AND {
		return x.placeProvenance(value.X)
	}
	return quality.Unknown
}

func (x *extractor) fieldOrigin(value *ast.SelectorExpr) quality.Provenance {
	object, _ := x.placeRoot(value)
	if object == nil {
		return quality.Unknown
	}
	if packageVariable(object) || x.parameters[object] {
		return quality.External
	}
	// Reference fields may contain aliases in locally allocated structs.
	return quality.Unknown
}

func (x *extractor) sliceOrigin(value *ast.SliceExpr, seen map[types.Object]bool) quality.Provenance {
	if valueArray(x.pkg.TypesInfo.TypeOf(value.X)) {
		return x.placeProvenance(value.X)
	}
	return x.origin(value.X, seen)
}

func (x *extractor) allocationOrigin(call *ast.CallExpr) quality.Provenance {
	id, ok := unwrappedCallee(call.Fun).(*ast.Ident)
	if !ok {
		return quality.Unknown
	}
	builtin, ok := x.pkg.TypesInfo.ObjectOf(id).(*types.Builtin)
	// append is locally owned: it either reuses the first argument's backing
	// array (whose provenance is tracked separately through the binding values)
	// or allocates a fresh one. It never introduces external sharing by itself.
	if ok && (builtin.Name() == "make" || builtin.Name() == "new" || builtin.Name() == "append") {
		return quality.Local
	}
	return quality.Unknown
}

func (x *extractor) objectOrigin(object types.Object, seen map[types.Object]bool) quality.Provenance {
	if object == nil {
		return quality.Unknown
	}
	if packageVariable(object) {
		return quality.External
	}
	if x.parameters[object] {
		return quality.External
	}
	if seen[object] {
		return quality.Unknown
	}
	binding, ok := x.bindings[object]
	if !ok {
		return quality.Unknown
	}
	seen[object] = true
	defer delete(seen, object)
	return x.bindingOrigin(binding, seen)
}

func (x *extractor) bindingOrigin(binding referenceBinding, seen map[types.Object]bool) quality.Provenance {
	if binding.unknown {
		return quality.Unknown
	}
	if len(binding.values) == 0 {
		return quality.Local
	}
	result := x.origin(binding.values[0], seen)
	for _, value := range binding.values[1:] {
		if result != x.origin(value, seen) {
			return quality.Unknown
		}
	}
	return result
}

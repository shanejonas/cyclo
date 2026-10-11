package gopatterns

import (
	"github.com/shanejonas/cyclo/domain/pdg"
	"go/types"
)

// Count distinct recorded variable bindings, not uses, aliases or locations.
// Go's reference-bearing value kinds define the language-specific policy.
func (b *irBuilder) references() {
	policy := b.pool.Policy("cyclo.go-reference-bindings", "1")
	count := pdg.CountFact{Status: pdg.Known, Policy: policy}
	for object := range b.symbols {
		variable, ok := referenceVariable(object)
		if !ok {
			continue
		}
		typ := types.Unalias(variable.Type())
		if _, generic := typ.(*types.TypeParam); generic {
			count.Status = pdg.Unknown
			count.Reason = b.pool.Text("Reference classification of a type parameter is unresolved.")
			continue
		}
		if referenceType(typ) {
			count.Value++
		}
	}
	if count.Status != pdg.Known {
		count.Value = 0
	}
	b.graph.Function.ReferenceCount = count
}

func referenceType(typ types.Type) bool {
	switch typ.Underlying().(type) {
	case *types.Pointer, *types.Map, *types.Slice, *types.Chan, *types.Signature, *types.Interface:
		return true
	}
	basic, ok := typ.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.UnsafePointer
}

func referenceVariable(object types.Object) (*types.Var, bool) {
	variable, ok := object.(*types.Var)
	return variable, ok && !variable.IsField()
}

package goquality

import "go/types"

// A type parameter has value-array storage only if a constraint restricts all
// possible instances to arrays. Mixed array/reference unions stay conservative.
func valueArray(typ types.Type) bool {
	switch typ := underlying(typ).(type) {
	case *types.Array:
		return true
	case *types.Interface:
		return arrayRestriction(typ)
	case *types.Union:
		return arrayTerms(typ)
	default:
		return false
	}
}

func arrayRestriction(constraint *types.Interface) bool {
	// Embedded constraints intersect. One all-array restriction is sufficient;
	// a constraint with no such restriction cannot establish value ownership.
	for index := 0; index < constraint.NumEmbeddeds(); index++ {
		if valueArray(constraint.EmbeddedType(index)) {
			return true
		}
	}
	return false
}

func arrayTerms(union *types.Union) bool {
	for index := 0; index < union.Len(); index++ {
		if !valueArray(union.Term(index).Type()) {
			return false
		}
	}
	return union.Len() > 0
}

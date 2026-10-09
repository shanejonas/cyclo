package anemic

// Order is an anemic model: no methods, but 3 functions operate on it.
type Order struct {
	Items []string
	Total int
}

func CalculateTotal(o *Order) int {
	sum := 0
	for _, item := range o.Items {
		_ = item
		sum++
	}
	return sum
}

func ApplyDiscount(o *Order, pct int) {
	o.Total = o.Total * (100 - pct) / 100
}

func ValidateOrder(o Order) bool {
	return len(o.Items) > 0
}

// Healthy has a method, so it is not anemic.
type Healthy struct {
	Value int
}

func (h *Healthy) Get() int {
	return h.Value
}

func ProcessHealthy(h *Healthy) int {
	return h.Value * 2
}

func CheckHealthy(h Healthy) bool {
	return h.Value > 0
}

// Sparse has only 2 operating functions, so it is not anemic.
type Sparse struct {
	X int
}

func UseSparse1(s *Sparse) int {
	return s.X
}

func UseSparse2(s Sparse) int {
	return s.X + 1
}

// hidden is unexported, so it is not considered.
type hidden struct {
	Y int
}

func UseHidden1(h *hidden) int {
	return h.Y
}

func UseHidden2(h *hidden) int {
	return h.Y * 2
}

func UseHidden3(h hidden) int {
	return h.Y + 3
}

package patterns

// ccPairBatch transfers ownership of bounded match batches to the union worker.
// Sent slices are never reused while the consumer can still read them.
type ccPairBatch struct {
	pairs []ccPairJob
	found chan<- []ccPairJob
}

func (b *ccPairBatch) add(pair ccPairJob) {
	b.pairs = append(b.pairs, pair)
	if len(b.pairs) < cap(b.pairs) {
		return
	}
	b.flush()
	b.pairs = make([]ccPairJob, 0, 64)
}

func (b *ccPairBatch) flush() {
	if len(b.pairs) == 0 {
		return
	}
	b.found <- b.pairs
	b.pairs = nil
}

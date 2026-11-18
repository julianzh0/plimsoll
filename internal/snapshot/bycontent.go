package snapshot

import (
	"crypto/sha256"
	"maps"
)

// byContent keeps values made from one content, by a hash of what they depend on, with the latest question that used
// each: a later question finds them, and the end of a question drops those no question since used. Its owner's lock
// guards it.
type byContent[V any] struct {
	kept map[[sha256.Size]byte]*keptValue[V]
}

type keptValue[V any] struct {
	value    V
	question uint64
}

// contentKey is the key of a value made from a content for one purpose and entry, hashed without a copy of the content.
func contentKey(purpose string, content []byte) [sha256.Size]byte {
	h := sha256.New()
	_, _ = h.Write([]byte(purpose)) // a hash's Write never fails
	_, _ = h.Write(content)
	var key [sha256.Size]byte
	h.Sum(key[:0])
	return key
}

// drop drops, when more values are kept than the bound, those that no question from this one on has used.
func (c *byContent[V]) drop(question uint64, bound int) {
	if len(c.kept) > bound {
		maps.DeleteFunc(c.kept, func(_ [sha256.Size]byte, kept *keptValue[V]) bool { return kept.question < question })
	}
}

// keptOrMade gives the value kept under a key, or makes it outside the lock and keeps it when made says so. Two
// questions that need the same value at the same time may both make it; the values are equal.
func keptOrMade[V any](rs *Readings, c *byContent[V], key [sha256.Size]byte, question uint64, made func() (V, bool)) V {
	rs.mu.Lock()
	if kept, found := c.kept[key]; found {
		kept.question = max(kept.question, question)
		rs.mu.Unlock()
		return kept.value
	}
	rs.mu.Unlock()
	value, keep := made()
	if keep {
		rs.mu.Lock()
		if c.kept == nil {
			c.kept = map[[sha256.Size]byte]*keptValue[V]{}
		}
		c.kept[key] = &keptValue[V]{value: value, question: question}
		rs.mu.Unlock()
	}
	return value
}

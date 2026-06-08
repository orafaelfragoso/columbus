package embed

import "math"

// clsVector extracts the [CLS] token (position 0) embedding for row r from a
// flattened [rows, seq, dim] last_hidden_state buffer. bge-small-en-v1.5 pools
// the CLS token rather than mean-pooling.
func clsVector(hidden []float32, row, seq, dim int) []float32 {
	base := row * seq * dim
	v := make([]float32, dim)
	copy(v, hidden[base:base+dim])
	return v
}

// l2normalize scales v in place to unit length, so a dot product of two
// outputs equals their cosine similarity. A zero vector is left unchanged.
func l2normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return
	}
	inv := float32(1.0 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
}

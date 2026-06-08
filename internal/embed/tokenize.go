package embed

import (
	"github.com/daulet/tokenizers"
)

// maxSeqLen caps tokens per text. bge-small-en-v1.5 accepts up to 512; the
// tokenizer truncates from the right, preserving the leading [CLS] we pool.
const maxSeqLen = 512

// tokenizer wraps the Hugging Face tokenizer loaded from the embedded
// tokenizer.json, giving the ONNX model byte-for-byte tokenization parity.
type tokenizer struct {
	tk *tokenizers.Tokenizer
}

// batch holds the three int64 [rows, seqLen] tensors the model consumes, each
// flattened row-major and right-padded to seqLen (PAD id 0, mask 0, type 0).
type batch struct {
	ids    []int64
	mask   []int64
	types  []int64
	rows   int
	seqLen int
}

func newTokenizer(data []byte) (*tokenizer, error) {
	tk, err := tokenizers.FromBytesWithTruncation(data, maxSeqLen, tokenizers.TruncationDirectionRight)
	if err != nil {
		return nil, embedFailure("load tokenizer: %v", err)
	}
	return &tokenizer{tk: tk}, nil
}

func (t *tokenizer) Close() {
	if t.tk != nil {
		t.tk.Close()
		t.tk = nil
	}
}

// encodeBatch tokenizes every text (with special tokens, attention mask and
// type ids), then pads each row to the batch-max length.
func (t *tokenizer) encodeBatch(texts []string) (*batch, error) {
	encs := make([]tokenizers.Encoding, len(texts))
	seqLen := 1 // never feed a zero-width sequence
	for i, s := range texts {
		enc, err := t.tk.EncodeWithOptionsErr(s, true,
			tokenizers.WithReturnAttentionMask(),
			tokenizers.WithReturnTypeIDs())
		if err != nil {
			return nil, embedFailure("encode text %d: %v", i, err)
		}
		encs[i] = enc
		if len(enc.IDs) > seqLen {
			seqLen = len(enc.IDs)
		}
	}

	b := &batch{
		ids:    make([]int64, len(texts)*seqLen),
		mask:   make([]int64, len(texts)*seqLen),
		types:  make([]int64, len(texts)*seqLen),
		rows:   len(texts),
		seqLen: seqLen,
	}
	for i, enc := range encs {
		base := i * seqLen
		for j := range enc.IDs {
			b.ids[base+j] = int64(enc.IDs[j])
			b.mask[base+j] = int64(enc.AttentionMask[j])
			b.types[base+j] = int64(enc.TypeIDs[j])
		}
		// Trailing positions stay zero: PAD id 0, masked out, type 0.
	}
	return b, nil
}

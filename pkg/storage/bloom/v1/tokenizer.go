package v1

import (
	"unicode/utf8"
)

const (
	MaxRuneLen = 4
)

func reassemble(buf []rune, ln, pos int, result []byte) []byte {
	result = result[:0] // Reset the result slice
	for i := 0; i < ln; i++ {
		cur := pos % len(buf)
		pos++
		result = utf8.AppendRune(result, buf[cur])
	}
	return result
}

// Iterable variants (more performant, less space)
type NGramTokenizer struct {
	N, Skip int
	buffer  []rune // circular buffer used for ngram generation
	res     []byte // buffer used for token generation
}

/*
N-Grams (https://en.wikipedia.org/wiki/N-gram) are a series of 'n' adjacent characters in a string.
These will be utilized for the bloom filters to allow for fuzzy searching.
*/
func NewNGramTokenizer(n, skip int) *NGramTokenizer {
	t := &NGramTokenizer{
		N:      n,
		Skip:   skip,
		buffer: make([]rune, n+skip),
		res:    make([]byte, 0, n*MaxRuneLen), // maximum 4 bytes per rune
	}

	return t
}

// The Token iterator uses shared buffers for performance. The []byte returned by At()
// is not safe for use after subsequent calls to Next()
func (t *NGramTokenizer) Tokens(line string) NGramTokenIter {
	return NGramTokenIter{
		n:    t.N,
		skip: t.Skip,

		line: line,

		buffer: t.buffer,
		res:    t.res,
	}
}

type NGramTokenIter struct {
	n, skip int

	runeIndex, offset int
	line              string // source

	buffer []rune // circular buffers used for ngram generation
	res    []byte
}

func (t *NGramTokenIter) Next() bool {
	for i, r := range t.line[t.offset:] {
		t.buffer[t.runeIndex%len(t.buffer)] = r
		t.runeIndex++

		if t.runeIndex < t.n {
			continue
		}

		// if the start of the ngram is at the interval of our skip factor, emit it.
		// we increment the skip due to modulo logic:
		//   because `n % 0 is a divide by zero and n % 1 is always 0`
		if (t.runeIndex-t.n)%(t.skip+1) == 0 {
			t.offset += (i + utf8.RuneLen(r))
			return true
		}

	}
	return false
}

func (t *NGramTokenIter) At() []byte {
	return reassemble(t.buffer, t.n, (t.runeIndex-t.n)%len(t.buffer), t.res[:0])
}

func (t *NGramTokenIter) Err() error {
	return nil
}

type PrefixedTokenIter struct {
	buf       []byte
	prefixLen int

	NGramTokenIter
}

func (t *PrefixedTokenIter) At() []byte {
	return append(t.buf[:t.prefixLen], t.NGramTokenIter.At()...)
}

func NewPrefixedTokenIter(buf []byte, prefixLn int, iter NGramTokenIter) *PrefixedTokenIter {
	return &PrefixedTokenIter{
		buf:            buf,
		prefixLen:      prefixLn,
		NGramTokenIter: iter,
	}
}

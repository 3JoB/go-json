package decoder

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"unsafe"

	"github.com/3JoB/go-json/internal/errors"
	"github.com/3JoB/unsafeConvert"
)

const (
	initBufSize = 512
)

type Stream struct {
	r                     io.Reader
	buf                   []byte
	bufSize               int64
	length                int64
	offset                int64
	cursor                int64
	filledBuffer          bool
	allRead               bool
	UseNumber             bool
	DisallowUnknownFields bool
	Option                *Option
}

func NewStream(r io.Reader) *Stream {
	return &Stream{
		r:       r,
		bufSize: initBufSize,
		buf:     make([]byte, initBufSize),
		Option:  &Option{},
	}
}

func (s *Stream) TotalOffset() int64 {
	return s.totalOffset()
}

func (s *Stream) Buffered() io.Reader {
	buflen := int64(len(s.buf))
	for i := s.cursor; i < buflen; i++ {
		if s.buf[i] == nul {
			return bytes.NewReader(s.buf[s.cursor:i])
		}
	}
	return bytes.NewReader(s.buf[s.cursor:])
}

func (s *Stream) PrepareForDecode() error {
	for {
		switch s.char() {
		case ' ', '\t', '\r', '\n':
			s.cursor++
			continue
		case ',', ':':
			s.cursor++
			return nil
		case nul:
			if s.read() {
				continue
			}
			return io.EOF
		}
		break
	}
	return nil
}

func (s *Stream) totalOffset() int64 {
	return s.offset + s.cursor
}

func (s *Stream) char() byte {
	return s.buf[s.cursor]
}

func (s *Stream) equalChar(c byte) bool {
	cur := s.buf[s.cursor]
	if cur == nul {
		s.read()
		cur = s.buf[s.cursor]
	}
	return cur == c
}

func (s *Stream) stat() ([]byte, int64, unsafe.Pointer) {
	return s.buf, s.cursor, (*sliceHeader)(unsafe.Pointer(&s.buf)).data
}

func (s *Stream) bufptr() unsafe.Pointer {
	return (*sliceHeader)(unsafe.Pointer(&s.buf)).data
}

func (s *Stream) statForRetry() ([]byte, int64, unsafe.Pointer) {
	s.cursor-- // for retry ( because caller progress cursor position in each loop )
	return s.buf, s.cursor, (*sliceHeader)(unsafe.Pointer(&s.buf)).data
}

func (s *Stream) Reset() {
	s.reset()
}

func (s *Stream) More() bool {
	for {
		switch s.char() {
		case ' ', '\n', '\r', '\t':
			s.cursor++
			continue
		case '}', ']':
			return false
		case nul:
			if s.read() {
				continue
			}
			return false
		}
		break
	}
	return true
}

func (s *Stream) Token() (any, error) {
	for {
		c := s.char()
		switch c {
		case ' ', '\n', '\r', '\t':
			s.cursor++
		case '{', '[', ']', '}':
			s.cursor++
			return json.Delim(c), nil
		case ',', ':':
			s.cursor++
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			bytes := floatBytes(s)
			str := *(*string)(unsafe.Pointer(&bytes))
			if s.UseNumber {
				return json.Number(str), nil
			}
			f64, err := strconv.ParseFloat(str, 64)
			if err != nil {
				return nil, err
			}
			return f64, nil
		case '"':
			bytes, cursor, err := stringBytes(s)
			s.cursor = cursor
			if err != nil {
				return nil, err
			}
			return unsafeConvert.StringReflect(bytes), nil
		case 't':
			if err := trueBytes(s); err != nil {
				return nil, err
			}
			return true, nil
		case 'f':
			if err := falseBytes(s); err != nil {
				return nil, err
			}
			return false, nil
		case 'n':
			if err := nullBytes(s); err != nil {
				return nil, err
			}
			return nil, nil
		case nul:
			if s.read() {
				continue
			}
			goto END
		default:
			return nil, errors.ErrInvalidCharacter(s.char(), "token", s.totalOffset())
		}
	}
END:
	return nil, io.EOF
}

func (s *Stream) reset() {
	s.offset += s.cursor
	s.buf = s.buf[s.cursor:]
	s.length -= s.cursor
	s.cursor = 0
}

func (s *Stream) readBuf() []byte {
	if s.filledBuffer {
		s.bufSize *= 2
		remainBuf := s.buf
		s.buf = make([]byte, s.bufSize)
		copy(s.buf, remainBuf)
	}
	return s.buf[s.length:]
}

func (s *Stream) read() bool {
	if s.allRead {
		return false
	}
	buf := s.readBuf()
	last := len(buf) - 1
	n, err := s.r.Read(buf[:last])
	buf[n] = nul
	s.length += int64(n)
	if n == last {
		s.filledBuffer = true
	} else {
		s.filledBuffer = false
	}
	if err == io.EOF {
		s.allRead = true
	} else if err != nil {
		return false
	}
	return true
}

// form: https://github.com/goccy/go-json/pull/446
func (s *Stream) requires(cursor, n int64) (read int) {
RETRY:
	if s.length-cursor < n {
		if !s.read() {
			return -1
		}
		read++
		goto RETRY
	}
	return
}

// form: https://github.com/goccy/go-json/pull/446
func (s *Stream) syncBufptr(r int, p *unsafe.Pointer) int {
	if r > 0 {
		*p = s.bufptr()
	}
	return r
}

func (s *Stream) skipWhiteSpace() byte {
	p := s.bufptr()
LOOP:
	c := char(p, s.cursor)
	switch c {
	case ' ', '\n', '\t', '\r':
		s.cursor++
		goto LOOP
	case nul:
		if s.read() {
			p = s.bufptr()
			goto LOOP
		}
	}
	return c
}

func (s *Stream) skipObject(depth int64) error {
	braceCount := 1
	_, cursor, p := s.stat()
	for {
		switch char(p, cursor) {
		case '{':
			braceCount++
			depth++
			if depth > maxDecodeNestingDepth {
				return errors.ErrExceededMaxDepth(s.char(), s.cursor)
			}
		case '}':
			braceCount--
			depth--
			if braceCount == 0 {
				s.cursor = cursor + 1
				return nil
			}
		case '[':
			depth++
			if depth > maxDecodeNestingDepth {
				return errors.ErrExceededMaxDepth(s.char(), s.cursor)
			}
		case ']':
			depth--
		case '"':
			for {
				cursor++
				switch char(p, cursor) {
				case '\\':
					cursor++
					if char(p, cursor) == nul {
						s.cursor = cursor
						if s.read() {
							_, cursor, p = s.stat()
							continue
						}
						return errors.ErrUnexpectedEndOfJSON("string of object", cursor)
					}
				case '"':
					goto SWITCH_OUT
				case nul:
					s.cursor = cursor
					if s.read() {
						_, cursor, p = s.statForRetry()
						continue
					}
					return errors.ErrUnexpectedEndOfJSON("string of object", cursor)
				}
			}
		case nul:
			s.cursor = cursor
			if s.read() {
				_, cursor, p = s.stat()
				continue
			}
			return errors.ErrUnexpectedEndOfJSON("object of object", cursor)
		}
	SWITCH_OUT:
		cursor++
	}
}

func (s *Stream) skipArray(depth int64) error {
	bracketCount := 1
	_, cursor, p := s.stat()
	for {
		switch char(p, cursor) {
		case '[':
			bracketCount++
			depth++
			if depth > maxDecodeNestingDepth {
				return errors.ErrExceededMaxDepth(s.char(), s.cursor)
			}
		case ']':
			bracketCount--
			depth--
			if bracketCount == 0 {
				s.cursor = cursor + 1
				return nil
			}
		case '{':
			depth++
			if depth > maxDecodeNestingDepth {
				return errors.ErrExceededMaxDepth(s.char(), s.cursor)
			}
		case '}':
			depth--
		case '"':
			for {
				cursor++
				switch char(p, cursor) {
				case '\\':
					cursor++
					if char(p, cursor) == nul {
						s.cursor = cursor
						if s.read() {
							_, cursor, p = s.stat()
							continue
						}
						return errors.ErrUnexpectedEndOfJSON("string of object", cursor)
					}
				case '"':
					goto SWITCH_OUT
				case nul:
					s.cursor = cursor
					if s.read() {
						_, cursor, p = s.statForRetry()
						continue
					}
					return errors.ErrUnexpectedEndOfJSON("string of object", cursor)
				}
			}
		case nul:
			s.cursor = cursor
			if s.read() {
				_, cursor, p = s.stat()
				continue
			}
			return errors.ErrUnexpectedEndOfJSON("array of object", cursor)
		}
	SWITCH_OUT:
		cursor++
	}
}

func (s *Stream) SkipErrorValue() {
	_ = s.skipValue(0)
}

func (s *Stream) skipValue(depth int64) error {
	_, cursor, p := s.stat()
	for {
		switch char(p, cursor) {
		case ' ', '\n', '\t', '\r':
			cursor++
			continue
		case nul:
			s.cursor = cursor
			if s.read() {
				_, cursor, p = s.stat()
				continue
			}
			return errors.ErrUnexpectedEndOfJSON("value of object", s.totalOffset())
		case '{':
			s.cursor = cursor + 1
			return s.skipObject(depth + 1)
		case '[':
			s.cursor = cursor + 1
			return s.skipArray(depth + 1)
		case '"':
			for {
				cursor++
				switch char(p, cursor) {
				case '\\':
					cursor++
					if char(p, cursor) == nul {
						s.cursor = cursor
						if s.read() {
							_, cursor, p = s.stat()
							continue
						}
						return errors.ErrUnexpectedEndOfJSON("value of string", s.totalOffset())
					}
				case '"':
					s.cursor = cursor + 1
					return nil
				case nul:
					s.cursor = cursor
					if s.read() {
						_, cursor, p = s.statForRetry()
						continue
					}
					return errors.ErrUnexpectedEndOfJSON("value of string", s.totalOffset())
				}
			}
		case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			for {
				cursor++
				c := char(p, cursor)
				if floatTable[c] {
					continue
				} else if c == nul {
					if s.read() {
						_, cursor, p = s.stat()
						continue
					}
				}
				s.cursor = cursor
				return nil
			}
		case 't':
			s.cursor = cursor
			if err := trueBytes(s); err != nil {
				return err
			}
			return nil
		case 'f':
			s.cursor = cursor
			if err := falseBytes(s); err != nil {
				return err
			}
			return nil
		case 'n':
			s.cursor = cursor
			if err := nullBytes(s); err != nil {
				return err
			}
			return nil
		}
		cursor++
	}
}

func nullBytes(s *Stream) error {
	if s.requires(s.cursor, 4) < 0 {
		s.cursor = s.length
		return errors.ErrUnexpectedEndOfJSON("null", s.cursor)
	}
	// current cursor's character is 'n'
	s.cursor++
	if s.char() != 'u' {
		return errors.ErrInvalidCharacter(s.char(), "null", s.totalOffset())
	}
	s.cursor++
	if s.char() != 'l' {
		return errors.ErrInvalidCharacter(s.char(), "null", s.totalOffset())
	}
	s.cursor++
	if s.char() != 'l' {
		return errors.ErrInvalidCharacter(s.char(), "null", s.totalOffset())
	}
	s.cursor++
	return nil
}

func trueBytes(s *Stream) error {
	if s.requires(s.cursor, 4) < 0 {
		s.cursor = s.length
		return errors.ErrUnexpectedEndOfJSON("bool(true)", s.cursor)
	}
	// current cursor's character is 't'
	s.cursor++
	if s.char() != 'r' {
		return errors.ErrInvalidCharacter(s.char(), "bool(true)", s.totalOffset())
	}
	s.cursor++
	if s.char() != 'u' {
		return errors.ErrInvalidCharacter(s.char(), "bool(true)", s.totalOffset())
	}
	s.cursor++
	if s.char() != 'e' {
		return errors.ErrInvalidCharacter(s.char(), "bool(true)", s.totalOffset())
	}
	s.cursor++
	return nil
}

func falseBytes(s *Stream) error {
	if s.requires(s.cursor, 5) < 0 {
		s.cursor = s.length
		return errors.ErrUnexpectedEndOfJSON("bool(false)", s.cursor)
	}
	// current cursor's character is 'f'
	s.cursor++
	if s.char() != 'a' {
		return errors.ErrInvalidCharacter(s.char(), "bool(false)", s.totalOffset())
	}
	s.cursor++
	if s.char() != 'l' {
		return errors.ErrInvalidCharacter(s.char(), "bool(false)", s.totalOffset())
	}
	s.cursor++
	if s.char() != 's' {
		return errors.ErrInvalidCharacter(s.char(), "bool(false)", s.totalOffset())
	}
	s.cursor++
	if s.char() != 'e' {
		return errors.ErrInvalidCharacter(s.char(), "bool(false)", s.totalOffset())
	}
	s.cursor++
	return nil
}

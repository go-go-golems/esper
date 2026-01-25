package parse

import "bytes"

type LineSplitter struct {
	tail []byte
}

func (s *LineSplitter) Push(chunk []byte) (lines [][]byte) {
	if len(chunk) == 0 {
		return nil
	}

	buf := chunk
	if len(s.tail) > 0 {
		buf = append(append([]byte{}, s.tail...), chunk...)
		s.tail = nil
	}

	start := 0
	for start < len(buf) {
		i := bytes.IndexByte(buf[start:], '\n')
		if i < 0 {
			break
		}
		end := start + i + 1
		lines = append(lines, buf[start:end])
		start = end
	}

	if start < len(buf) {
		s.tail = append([]byte{}, buf[start:]...)
	}

	return lines
}

func (s *LineSplitter) Tail() []byte {
	return append([]byte{}, s.tail...)
}

func (s *LineSplitter) FinalizeTail() []byte {
	if len(s.tail) == 0 {
		return nil
	}
	out := s.tail
	s.tail = nil
	return out
}


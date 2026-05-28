package decode

import "bytes"

type GDBStubDetector struct {
	tail []byte
}

type GDBStubMatch struct {
	Payload []byte // bytes between '$' and '#', inclusive of leading 'T..'
}

func (d *GDBStubDetector) Push(chunk []byte) (match *GDBStubMatch) {
	if len(chunk) == 0 {
		return nil
	}
	buf := chunk
	if len(d.tail) > 0 {
		buf = append(append([]byte{}, d.tail...), chunk...)
	}

	// Look for: $T..#..
	for i := 0; i+7 <= len(buf); i++ {
		if buf[i] != '$' || buf[i+1] != 'T' {
			continue
		}
		hash := bytes.IndexByte(buf[i+1:], '#')
		if hash < 0 {
			continue
		}
		hash = i + 1 + hash
		if hash+2 >= len(buf) {
			continue
		}
		payload := buf[i+1 : hash] // exclude '$', exclude '#'
		chk := buf[hash+1 : hash+3]
		if checksumOK(payload, chk) {
			return &GDBStubMatch{Payload: payload}
		}
	}

	// keep last 6 bytes (ESP-IDF monitor keeps 6 for 7-byte sequences)
	if len(buf) > 6 {
		d.tail = append([]byte{}, buf[len(buf)-6:]...)
	} else {
		d.tail = append([]byte{}, buf...)
	}
	return nil
}

func checksumOK(payload []byte, chkHex []byte) bool {
	if len(chkHex) != 2 {
		return false
	}
	var got byte
	for _, b := range chkHex {
		got <<= 4
		switch {
		case b >= '0' && b <= '9':
			got |= b - '0'
		case b >= 'a' && b <= 'f':
			got |= 10 + (b - 'a')
		case b >= 'A' && b <= 'F':
			got |= 10 + (b - 'A')
		default:
			return false
		}
	}

	var sum byte
	for _, b := range payload {
		sum += b
	}
	return sum == got
}

package questie

import (
	"errors"
	"math"
)

// decodeCBOR reads one CBOR value from b. Integers become int64, floats float64, byte and text
// strings string, arrays []any, maps map[any]any (keys int64 or string), null nil. Only the shapes
// the database stores are handled; anything else is an error, never a guess.
func decodeCBOR(b []byte) (any, error) {
	v, n, err := cborValue(b, 0, 0)
	if err != nil {
		return nil, err
	}
	if n != len(b) {
		return nil, errors.New("trailing bytes after value")
	}
	return v, nil
}

const maxDepth, maxItems = 12, 1 << 20

var errShort = errors.New("truncated value")

func cborValue(b []byte, i, depth int) (any, int, error) {
	if depth > maxDepth {
		return nil, 0, errors.New("nested too deep")
	}
	if i >= len(b) {
		return nil, 0, errShort
	}
	major, info := b[i]>>5, b[i]&31
	i++
	var n uint64
	switch {
	case info < 24:
		n = uint64(info)
	case info >= 24 && info <= 27:
		w := 1 << (info - 24)
		if i+w > len(b) {
			return nil, 0, errShort
		}
		for _, c := range b[i : i+w] {
			n = n<<8 | uint64(c)
		}
		i += w
	default:
		return nil, 0, errors.New("unsupported encoding")
	}
	switch major {
	case 0:
		if n > math.MaxInt64 {
			return nil, 0, errors.New("integer too large")
		}
		return int64(n), i, nil
	case 1:
		if n > math.MaxInt64 {
			return nil, 0, errors.New("integer too large")
		}
		return -1 - int64(n), i, nil
	case 2, 3:
		if n > uint64(len(b)-i) {
			return nil, 0, errShort
		}
		return string(b[i : i+int(n)]), i + int(n), nil
	case 4, 5:
		per := 1 + int(major-4)
		if n > maxItems {
			return nil, 0, errors.New("collection too large")
		}
		var arr []any
		m := map[any]any{}
		for c := uint64(0); c < n; c++ {
			var k, v any
			var err error
			for p := 0; p < per; p++ {
				var x any
				if x, i, err = cborValue(b, i, depth+1); err != nil {
					return nil, 0, err
				}
				if p == 0 {
					k = x
				} else {
					v = x
				}
			}
			if major == 4 {
				arr = append(arr, k)
			} else {
				switch k.(type) {
				case int64, string:
				default:
					return nil, 0, errors.New("unsupported map key")
				}
				m[k] = v
			}
		}
		if major == 4 {
			if arr == nil {
				arr = []any{}
			}
			return arr, i, nil
		}
		return m, i, nil
	default: // 7: simple values and floats
		switch info {
		case 20:
			return false, i, nil
		case 21:
			return true, i, nil
		case 22, 23:
			return nil, i, nil
		case 25:
			return half(uint16(n)), i, nil
		case 26:
			return float64(math.Float32frombits(uint32(n))), i, nil
		case 27:
			return math.Float64frombits(n), i, nil
		}
		return nil, 0, errors.New("unsupported simple value")
	}
}

func half(h uint16) float64 {
	sign, exp, frac := float64(1), int(h>>10)&31, float64(h&1023)
	if h&0x8000 != 0 {
		sign = -1
	}
	switch exp {
	case 0:
		return sign * frac / 1024 * math.Pow(2, -14)
	case 31:
		return sign * math.Inf(1)
	}
	return sign * (1 + frac/1024) * math.Pow(2, float64(exp-15))
}

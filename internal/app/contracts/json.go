package contracts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
)

// The JSON boundary policy. Every shell-facing payload passes through Encode
// or Decode so that size, depth, unknown-field and safe-integer rules are
// enforced in exactly one place.
const (
	// MaxPayloadBytes bounds one encoded bridge payload (1 MiB, matching the
	// terminal data-plane frame bound; NOT a terminal byte-frame contract).
	MaxPayloadBytes = 1 << 20
	// MaxDepth bounds JSON nesting depth.
	MaxDepth = 32
	// SafeIntegerMax is the largest integer exactly representable in an
	// IEEE 754 double (JavaScript Number.MAX_SAFE_INTEGER).
	SafeIntegerMax = int64(1)<<53 - 1
	// SafeIntegerMin is the smallest exactly representable integer.
	SafeIntegerMin = -SafeIntegerMax
)

var (
	// ErrPayloadTooLarge rejects oversized payloads.
	ErrPayloadTooLarge = errors.New("contracts: payload exceeds bound")
	// ErrDepthExceeded rejects nesting beyond the depth bound.
	ErrDepthExceeded = errors.New("contracts: payload depth exceeds bound")
	// ErrUnsafeInteger rejects integers outside the JavaScript safe range.
	ErrUnsafeInteger = errors.New("contracts: integer outside safe range")
	// ErrUnknownField rejects payloads carrying fields the target type does
	// not declare.
	ErrUnknownField = errors.New("contracts: unknown field")
)

// Encode serialises value with the full boundary policy applied: safe
// integers only, bounded size.
func Encode(value any) ([]byte, error) {
	if err := checkSafeIntegers(reflect.ValueOf(value), 0); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(encoded) > MaxPayloadBytes {
		return nil, fmt.Errorf("%w: %d > %d bytes", ErrPayloadTooLarge, len(encoded), MaxPayloadBytes)
	}
	return encoded, nil
}

// Decode parses data into out with the full boundary policy applied: bounded
// size, bounded depth, safe integers and unknown-field rejection.
func Decode(data []byte, out any) error {
	if len(data) > MaxPayloadBytes {
		return fmt.Errorf("%w: %d > %d bytes", ErrPayloadTooLarge, len(data), MaxPayloadBytes)
	}
	if err := checkDepth(data); err != nil {
		return err
	}
	if err := checkSafeIntegersRaw(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		message := err.Error()
		switch {
		case strings.Contains(message, "unknown field"):
			return fmt.Errorf("%w: %v", ErrUnknownField, err)
		default:
			return fmt.Errorf("contracts: decode failed: %w", err)
		}
	}
	return nil
}

func checkDepth(data []byte) error {
	depth := 0
	maxSeen := 0
	inString := false
	escaped := false
	for _, b := range data {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch b {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > maxSeen {
				maxSeen = depth
			}
			if depth > MaxDepth {
				return fmt.Errorf("%w: %d > %d", ErrDepthExceeded, depth, MaxDepth)
			}
		case '}', ']':
			depth--
		}
	}
	return nil
}

// checkSafeIntegersRaw scans numeric literals in encoded JSON for values that
// JavaScript cannot represent exactly.
func checkSafeIntegersRaw(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil // structural errors surface in Decode; not duplicated here
	}
	return checkNumbers(reflect.ValueOf(root), 0)
}

func checkSafeIntegers(value reflect.Value, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("%w: %d > %d", ErrDepthExceeded, depth, MaxDepth)
	}
	return checkNumbers(value, depth)
}

func checkNumbers(value reflect.Value, depth int) error {
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return checkNumbers(value.Elem(), depth)
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if err := checkNumbers(value.Field(i), depth+1); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range value.MapKeys() {
			if err := checkNumbers(value.MapIndex(key), depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if err := checkNumbers(value.Index(i), depth+1); err != nil {
				return err
			}
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number := value.Int()
		if number < SafeIntegerMin || number > SafeIntegerMax {
			return fmt.Errorf("%w: %d", ErrUnsafeInteger, number)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		number := value.Uint()
		if number > uint64(SafeIntegerMax) {
			return fmt.Errorf("%w: %d", ErrUnsafeInteger, number)
		}
	case reflect.Float32, reflect.Float64:
		number := value.Float()
		if number > math.MaxInt32 || number < math.MinInt32 {
			// Only flag values that lose integer exactness as numbers.
			if number != math.Trunc(number) || number > float64(SafeIntegerMax) || number < float64(SafeIntegerMin) {
				return fmt.Errorf("%w: %v", ErrUnsafeInteger, number)
			}
		}
	}
	return nil
}

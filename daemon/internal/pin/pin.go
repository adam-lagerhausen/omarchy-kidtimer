package pin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	timeCost    = 1
	memoryKiB   = 16 * 1024
	parallelism = 1
	keyLen      = 32
	saltLen     = 16
)

func ValidDigits(s string) bool {
	if len(s) != 4 {
		return false
	}
	for i := 0; i < 4; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func Hash(digits string) (string, error) {
	if !ValidDigits(digits) {
		return "", fmt.Errorf("pin must be 4 digits")
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(digits), salt, timeCost, memoryKiB, parallelism, keyLen)
	return encode(salt, sum), nil
}

func Verify(digits, encoded string) bool {
	if !ValidDigits(digits) || encoded == "" {
		return false
	}
	salt, want, ok := decode(encoded)
	if !ok {
		return false
	}
	got := argon2.IDKey([]byte(digits), salt, timeCost, memoryKiB, parallelism, keyLen)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func ValidEncoded(encoded string) bool {
	_, _, ok := decode(encoded)
	return ok
}

func encode(salt, sum []byte) string {
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memoryKiB, timeCost, parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	)
}

func decode(encoded string) (salt, sum []byte, ok bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return nil, nil, false
	}
	var m, t, p int
	n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p)
	if err != nil || n != 3 {
		return nil, nil, false
	}
	if m != memoryKiB || t != timeCost || p != parallelism {
		return nil, nil, false
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != saltLen {
		return nil, nil, false
	}
	sum, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(sum) != int(keyLen) {
		return nil, nil, false
	}
	return salt, sum, true
}

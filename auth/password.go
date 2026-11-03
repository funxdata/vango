package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"hash"
	"strconv"
	"strings"
)

const (
	passwordAlgorithm  = "pbkdf2-sha256"
	passwordIterations = 210000
	passwordKeyLength  = 32
	passwordSaltLength = 16
	// maxPasswordIterations 限制可接受的迭代次数，防止被篡改的哈希串造成 CPU 耗尽。
	maxPasswordIterations = 1_000_000
)

// HashPassword 使用 PBKDF2-SHA256 计算密码哈希。
//
// 输出格式为 pbkdf2-sha256$<iterations>$<salt>$<hash>，迭代次数随哈希一起保存，
// 便于后续在不破坏旧数据的前提下提升强度。
func HashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}

	key := pbkdf2Key([]byte(password), salt, passwordIterations, passwordKeyLength, sha256.New)
	return fmt.Sprintf("%s$%d$%s$%s",
		passwordAlgorithm,
		passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword 校验明文密码与哈希是否匹配，使用恒定时间比较。
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordAlgorithm {
		return false
	}

	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 || iterations > maxPasswordIterations {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) == 0 {
		return false
	}

	actual := pbkdf2Key([]byte(password), salt, iterations, len(expected), sha256.New)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func pbkdf2Key(password, salt []byte, iterations, keyLength int, newHash func() hash.Hash) []byte {
	prf := hmac.New(newHash, password)
	hashLength := prf.Size()
	blocks := (keyLength + hashLength - 1) / hashLength

	derived := make([]byte, 0, blocks*hashLength)
	var counter [4]byte

	for block := 1; block <= blocks; block++ {
		prf.Reset()
		prf.Write(salt)
		counter[0] = byte(block >> 24)
		counter[1] = byte(block >> 16)
		counter[2] = byte(block >> 8)
		counter[3] = byte(block)
		prf.Write(counter[:])

		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)

		for i := 1; i < iterations; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j := range t {
				t[j] ^= u[j]
			}
		}
		derived = append(derived, t...)
	}

	return derived[:keyLength]
}

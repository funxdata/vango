// Package auth 提供 JWT 签发/校验与密码哈希。
//
// 本包只依赖标准库：JWT 支持 HS256/HS384/HS512，密码哈希使用 PBKDF2-SHA256。
//
// 签发与校验：
//
//	signer, _ := auth.NewSigner([]byte("secret"), auth.WithIssuer("vango"))
//	token, _ := signer.Sign(auth.Claims{Subject: "42", Roles: []string{"admin"}}, time.Hour)
//	claims, err := signer.Parse(token)
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"strings"
	"time"

	"github.com/funxdata/vango/errs"
)

// Algorithm 是 JWT 的签名算法。
type Algorithm string

// 支持的 HMAC 签名算法。
const (
	HS256 Algorithm = "HS256"
	HS384 Algorithm = "HS384"
	HS512 Algorithm = "HS512"
)

var (
	// ErrInvalidToken 表示令牌格式错误或签名不匹配。
	ErrInvalidToken = errs.Unauthorized("invalid token")
	// ErrTokenExpired 表示令牌已过期。
	ErrTokenExpired = errs.Unauthorized("token expired")
	// ErrTokenNotYetValid 表示令牌尚未生效。
	ErrTokenNotYetValid = errs.Unauthorized("token not valid yet")
	// ErrMissingToken 表示请求未携带令牌。
	ErrMissingToken = errs.Unauthorized("missing token")
)

// Claims 是 vango 默认的 JWT 声明，业务可以通过匿名嵌入扩展字段：
//
//	type MyClaims struct {
//		auth.Claims
//		TenantID string `json:"tenant_id"`
//	}
type Claims struct {
	Subject   string   `json:"sub,omitempty"`
	Issuer    string   `json:"iss,omitempty"`
	Audience  string   `json:"aud,omitempty"`
	ID        string   `json:"jti,omitempty"`
	IssuedAt  int64    `json:"iat,omitempty"`
	NotBefore int64    `json:"nbf,omitempty"`
	ExpiresAt int64    `json:"exp,omitempty"`
	Roles     []string `json:"roles,omitempty"`
}

// Signer 负责签发与校验 JWT。
type Signer struct {
	secret []byte
	alg    Algorithm
	issuer string
	now    func() time.Time
}

// SignerOption 定制 Signer。
type SignerOption func(*Signer) error

// WithAlgorithm 指定签名算法，默认 HS256。
func WithAlgorithm(alg Algorithm) SignerOption {
	return func(s *Signer) error {
		switch alg {
		case HS256, HS384, HS512:
			s.alg = alg
			return nil
		default:
			return fmt.Errorf("auth: unsupported algorithm %q", alg)
		}
	}
}

// WithIssuer 指定签发的 iss 声明。
func WithIssuer(issuer string) SignerOption {
	return func(s *Signer) error {
		s.issuer = issuer
		return nil
	}
}

// WithClock 注入自定义时钟，便于测试。
func WithClock(now func() time.Time) SignerOption {
	return func(s *Signer) error {
		if now == nil {
			return fmt.Errorf("auth: clock must not be nil")
		}
		s.now = now
		return nil
	}
}

// NewSigner 创建一个 JWT 签发器。
func NewSigner(secret []byte, opts ...SignerOption) (*Signer, error) {
	if len(secret) == 0 {
		return nil, fmt.Errorf("auth: secret must not be empty")
	}

	s := &Signer{secret: secret, alg: HS256, now: time.Now}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Sign 签发令牌。ttl 为 0 时不写入 exp；claims 中已有的 iat/exp/iss 不会被覆盖。
//
// claims 可以是 auth.Claims，也可以是自定义结构体或 map[string]any。
func (s *Signer) Sign(claims any, ttl time.Duration) (string, error) {
	payload, err := toMap(claims)
	if err != nil {
		return "", err
	}

	now := s.now()
	if _, ok := payload["iat"]; !ok {
		payload["iat"] = now.Unix()
	}
	if ttl > 0 {
		if _, ok := payload["exp"]; !ok {
			payload["exp"] = now.Add(ttl).Unix()
		}
	}
	if s.issuer != "" {
		if _, ok := payload["iss"]; !ok {
			payload["iss"] = s.issuer
		}
	}

	headerJSON, err := json.Marshal(map[string]string{"alg": string(s.alg), "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	signingInput := encodeSegment(headerJSON) + "." + encodeSegment(payloadJSON)
	signature := s.sign([]byte(signingInput))
	return signingInput + "." + encodeSegment(signature), nil
}

// Parse 校验令牌并解析为标准 Claims。
func (s *Signer) Parse(token string) (*Claims, error) {
	claims := &Claims{}
	if err := s.ParseClaims(token, claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// ParseClaims 校验令牌并解析到自定义声明结构体。
func (s *Signer) ParseClaims(token string, dst any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ErrInvalidToken
	}

	headerJSON, err := decodeSegment(parts[0])
	if err != nil {
		return ErrInvalidToken
	}
	var header struct {
		Algorithm string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return ErrInvalidToken
	}
	if Algorithm(header.Algorithm) != s.alg {
		return ErrInvalidToken
	}

	signature, err := decodeSegment(parts[2])
	if err != nil {
		return ErrInvalidToken
	}
	expected := s.sign([]byte(parts[0] + "." + parts[1]))
	if subtle.ConstantTimeCompare(signature, expected) != 1 {
		return ErrInvalidToken
	}

	payloadJSON, err := decodeSegment(parts[1])
	if err != nil {
		return ErrInvalidToken
	}
	if err := json.Unmarshal(payloadJSON, dst); err != nil {
		return ErrInvalidToken
	}

	payload, err := toMap(dst)
	if err != nil {
		return ErrInvalidToken
	}
	return s.validateTimes(payload)
}

func (s *Signer) validateTimes(payload map[string]any) error {
	now := s.now().Unix()

	if exp, ok := toInt64(payload["exp"]); ok && exp > 0 && now > exp {
		return ErrTokenExpired
	}
	if nbf, ok := toInt64(payload["nbf"]); ok && nbf > 0 && now < nbf {
		return ErrTokenNotYetValid
	}
	return nil
}

func (s *Signer) sign(input []byte) []byte {
	mac := hmac.New(s.hash, s.secret)
	mac.Write(input)
	return mac.Sum(nil)
}

func (s *Signer) hash() hash.Hash {
	switch s.alg {
	case HS384:
		return sha512.New384()
	case HS512:
		return sha512.New()
	default:
		return sha256.New()
	}
}

func toMap(v any) (map[string]any, error) {
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any)
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func encodeSegment(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeSegment(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

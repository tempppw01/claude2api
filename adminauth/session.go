package adminauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	CookieName      = "admin_auth"
	SessionDuration = 7 * 24 * time.Hour
)

var sessionSecret = newSessionSecret()

func NewToken(adminPassword string) string {
	expiresAt := time.Now().Add(SessionDuration).Unix()
	payload := strconv.FormatInt(expiresAt, 10)
	signature := sign(payload, adminPassword)
	return payload + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func ValidateToken(token, adminPassword string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}

	expiresAt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > expiresAt {
		return false
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	expected := sign(parts[0], adminPassword)
	return hmac.Equal(signature, expected)
}

func sign(payload, adminPassword string) []byte {
	mac := hmac.New(sha256.New, sessionSecret)
	mac.Write([]byte(payload))
	mac.Write([]byte{0})
	mac.Write([]byte(adminPassword))
	return mac.Sum(nil)
}

func newSessionSecret() []byte {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err == nil {
		return secret
	}

	fallback := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return fallback[:]
}

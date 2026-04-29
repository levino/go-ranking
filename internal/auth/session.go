package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

const cookieName = "go_liga_session"

type SessionData struct {
	UserID int64 `json:"u"`
	Exp    int64 `json:"e"`
}

type Signer struct {
	Key []byte
}

func NewSigner(key []byte) *Signer { return &Signer{Key: key} }

func (s *Signer) sign(payload []byte) string {
	mac := hmac.New(sha256.New, s.Key)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Issue creates a cookie that authenticates the given user for the
// configured TTL.
func (s *Signer) Issue(w http.ResponseWriter, userID int64, ttl time.Duration) error {
	d := SessionData{UserID: userID, Exp: time.Now().Add(ttl).Unix()}
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	value := body + "." + s.sign([]byte(body))
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
	return nil
}

func (s *Signer) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// Read returns the user ID from a valid signed cookie, or an error
// if missing/forged/expired.
func (s *Signer) Read(r *http.Request) (int64, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return 0, err
	}
	parts := splitOnce(c.Value, '.')
	if len(parts) != 2 {
		return 0, errors.New("malformed cookie")
	}
	expected := s.sign([]byte(parts[0]))
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return 0, errors.New("bad signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, err
	}
	var d SessionData
	if err := json.Unmarshal(raw, &d); err != nil {
		return 0, err
	}
	if time.Now().Unix() > d.Exp {
		return 0, errors.New("expired")
	}
	return d.UserID, nil
}

func splitOnce(s string, sep byte) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

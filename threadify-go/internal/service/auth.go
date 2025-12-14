package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AuthService struct {
	secret     []byte
	issuer     string
	audience   string
	expiration time.Duration
}

func NewAuthService(secret, issuer, audience string, expirationHours int) *AuthService {
	return &AuthService{
		secret:     []byte(secret),
		issuer:     issuer,
		audience:   audience,
		expiration: time.Duration(expirationHours) * time.Hour,
	}
}

func (s *AuthService) CreateToken(userID string, claims map[string]interface{}) (string, error) {
	now := time.Now()
	jwtClaims := jwt.MapClaims{
		"sub": userID,
		"iss": s.issuer,
		"aud": s.audience,
		"iat": now.Unix(),
		"exp": now.Add(s.expiration).Unix(),
	}

	for k, v := range claims {
		jwtClaims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	return token.SignedString(s.secret)
}

func (s *AuthService) VerifyToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

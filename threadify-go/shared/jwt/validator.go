package jwt

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID    string   `json:"user_id"`
	CompanyID string   `json:"company_id"`
	Email     string   `json:"email"`
	Roles     []string `json:"roles,omitempty"`
	jwt.RegisteredClaims
}

type Validator struct {
	secret   []byte
	issuer   string
	audience string
}

func NewValidator(secret, issuer, audience string) *Validator {
	return &Validator{
		secret:   []byte(secret),
		issuer:   issuer,
		audience: audience,
	}
}

// CreateToken generates a new JWT token
func (v *Validator) CreateToken(userID, companyID, email string, roles []string, expiration time.Duration) (string, error) {
	claims := Claims{
		UserID:    userID,
		CompanyID: companyID,
		Email:     email,
		Roles:     roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    v.issuer,
			Audience:  jwt.ClaimStrings{v.audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(v.secret)
}

// ValidateToken validates and parses a JWT token
func (v *Validator) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.secret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

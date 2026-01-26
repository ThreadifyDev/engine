package utils

import "golang.org/x/crypto/bcrypt"

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

// CheckPassword compares a password with a hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// HashOTP hashes an OTP code using bcrypt (lower cost for performance)
func HashOTP(code string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(code), 10)
	return string(bytes), err
}

// CheckOTP compares an OTP code with a hash
func CheckOTP(code, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(code))
	return err == nil
}

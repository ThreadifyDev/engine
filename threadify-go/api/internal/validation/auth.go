package validation

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"threadify-go/api/internal/models"
)

const (
	maxEmailLength      = 254
	minPasswordLength   = 12
	maxPasswordLength   = 128
	minCompanyNameLen   = 2
	maxCompanyNameLen   = 255
	maxFullNameLen      = 120
	maxJobRoleLen       = 120
	maxIndustryLen      = 120
	maxUseCaseLen       = 500
	maxLoginPasswordLen = 256
)

var (
	emailPattern = regexp.MustCompile(`^[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	namePattern  = regexp.MustCompile(`^[\p{L}][\p{L} .'-]*$`)
	labelPattern = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} .,&'()\-+/]*$`)
)

var allowedCompanySizes = map[string]struct{}{
	"small":      {},
	"medium":     {},
	"large":      {},
	"enterprise": {},
}

var fieldLabels = map[string]string{
	"full_name": "Full name",
	"job_role":  "Job role",
	"industry":  "Industry",
	"use_case":  "Use case",
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type RequestValidationError struct {
	problems []FieldError
}

func (e *RequestValidationError) Error() string {
	if len(e.problems) == 0 {
		return "validation failed"
	}
	return "validation failed: " + e.problems[0].Message
}

func (e *RequestValidationError) Problems() []FieldError {
	if len(e.problems) == 0 {
		return nil
	}
	out := make([]FieldError, len(e.problems))
	copy(out, e.problems)
	return out
}

func (e *RequestValidationError) FirstMessage() string {
	if len(e.problems) == 0 {
		return "Invalid request"
	}
	return e.problems[0].Message
}

type validationBuilder struct {
	problems []FieldError
}

func (b *validationBuilder) add(field, message string) {
	b.problems = append(b.problems, FieldError{Field: field, Message: message})
}

func (b *validationBuilder) err() error {
	if len(b.problems) == 0 {
		return nil
	}
	return &RequestValidationError{problems: b.problems}
}

func ValidateSignupRequest(req *models.SignupRequest) error {
	b := &validationBuilder{}
	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}

	req.Email = normalizeEmail(req.Email)
	validateEmail("email", req.Email, b)

	req.CompanyName = strings.TrimSpace(req.CompanyName)
	validateCompanyName(req.CompanyName, b)
	validatePassword(req.Password, b)

	req.FullName = normalizeOptionalText("full_name", req.FullName, maxFullNameLen, namePattern, b)
	req.JobRole = normalizeOptionalText("job_role", req.JobRole, maxJobRoleLen, labelPattern, b)
	req.Industry = normalizeOptionalPointer("industry", req.Industry, maxIndustryLen, labelPattern, b)
	req.UseCase = normalizeOptionalPointer("use_case", req.UseCase, maxUseCaseLen, nil, b)
	req.CompanySize = normalizeCompanySize(req.CompanySize, b)

	return b.err()
}

func ValidateLoginRequest(req *models.LoginRequest) error {
	b := &validationBuilder{}
	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}

	req.Email = normalizeEmail(req.Email)
	validateEmail("email", req.Email, b)

	if req.Password == "" {
		b.add("password", "Password is required")
	} else if len(req.Password) > maxLoginPasswordLen {
		b.add("password", "Password exceeds maximum length")
	} else if hasControlChars(req.Password) {
		b.add("password", "Password contains invalid characters")
	}

	return b.err()
}

func ValidateForgotPasswordRequest(req *models.ForgotPasswordRequest) error {
	b := &validationBuilder{}
	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}
	req.Email = normalizeEmail(req.Email)
	validateEmail("email", req.Email, b)
	return b.err()
}

func ValidateResetPasswordRequest(req *models.ResetPasswordRequest) error {
	b := &validationBuilder{}
	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}
	if strings.TrimSpace(req.Token) == "" {
		b.add("token", "Reset token is required")
	}
	validatePassword(req.Password, b)
	return b.err()
}

func ValidateVerifyEmailRequest(req *models.VerifyEmailRequest) error {
	b := &validationBuilder{}
	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}
	req.Email = normalizeEmail(req.Email)
	validateEmail("email", req.Email, b)
	if strings.TrimSpace(req.Token) == "" {
		b.add("token", "Verification token is required")
	}
	return b.err()
}

func ValidateResendVerificationEmailRequest(req *models.ResendVerificationEmailRequest) error {
	b := &validationBuilder{}
	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}
	req.Email = normalizeEmail(req.Email)
	validateEmail("email", req.Email, b)
	return b.err()
}

func validateEmail(field, email string, b *validationBuilder) {
	switch {
	case email == "":
		b.add(field, "Email is required")
	case len(email) > maxEmailLength:
		b.add(field, "Email exceeds maximum length")
	case strings.ContainsAny(email, " \t\r\n"):
		b.add(field, "Email must not contain spaces")
	case !utf8.ValidString(email) || hasControlChars(email):
		b.add(field, "Email contains invalid characters")
	default:
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed == nil || !strings.EqualFold(parsed.Address, email) || !emailPattern.MatchString(email) {
			b.add(field, "Email format is invalid")
		}
	}
}

func validateCompanyName(name string, b *validationBuilder) {
	if name == "" {
		b.add("company_name", "Company name is required")
		return
	}
	runeLen := utf8.RuneCountInString(name)
	if runeLen < minCompanyNameLen || runeLen > maxCompanyNameLen {
		b.add("company_name", "Company name length is invalid")
		return
	}
	if hasControlChars(name) || !labelPattern.MatchString(name) {
		b.add("company_name", "Company name contains invalid characters")
	}
}

func validatePassword(password string, b *validationBuilder) {
	switch {
	case password == "":
		b.add("password", "Password is required")
		return
	case strings.TrimSpace(password) != password:
		b.add("password", "Password must not have leading or trailing spaces")
		return
	case strings.ContainsFunc(password, unicode.IsSpace):
		b.add("password", "Password must not contain whitespace")
		return
	case len(password) < minPasswordLength:
		b.add("password", "Password must be at least 12 characters")
		return
	case len(password) > maxPasswordLength:
		b.add("password", "Password exceeds maximum length")
		return
	case hasControlChars(password):
		b.add("password", "Password contains invalid characters")
		return
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSpecial = true
		}
		if hasUpper && hasLower && hasDigit && hasSpecial {
			break
		}
	}
	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		b.add("password", "Password must include upper, lower, number, and special character")
	}
}

// -- normalizers (unchanged) --

func normalizeOptionalText(field, value string, maxLen int, pattern *regexp.Regexp, b *validationBuilder) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !validateOptional(field, trimmed, maxLen, pattern, b) {
		return ""
	}
	return trimmed
}

func normalizeOptionalPointer(field string, value *string, maxLen int, pattern *regexp.Regexp, b *validationBuilder) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" || !validateOptional(field, trimmed, maxLen, pattern, b) {
		return nil
	}
	return &trimmed
}

func validateOptional(field, value string, maxLen int, pattern *regexp.Regexp, b *validationBuilder) bool {
	label := displayField(field)
	switch {
	case utf8.RuneCountInString(value) > maxLen:
		b.add(field, label+" exceeds maximum length")
		return false
	case hasControlChars(value):
		b.add(field, label+" contains invalid characters")
		return false
	case pattern != nil && !pattern.MatchString(value):
		b.add(field, label+" contains invalid characters")
		return false
	}
	return true
}

func normalizeCompanySize(value *string, b *validationBuilder) *string {
	if value == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized == "" {
		return nil
	}
	if _, ok := allowedCompanySizes[normalized]; !ok {
		b.add("company_size", "Company size must be one of: small, medium, large, enterprise")
		return nil
	}
	return &normalized
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func hasControlChars(v string) bool {
	for _, r := range v {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func displayField(field string) string {
	if label, ok := fieldLabels[field]; ok {
		return label
	}
	return field
}

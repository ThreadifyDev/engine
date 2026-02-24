package validation

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"threadify-go/api/internal/models"
	"unicode"
	"unicode/utf8"
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

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type RequestValidationError struct {
	problems []FieldError
}

func (e *RequestValidationError) Error() string {
	if e == nil || len(e.problems) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed: %s", e.problems[0].Message)
}

func (e *RequestValidationError) Problems() []FieldError {
	if e == nil || len(e.problems) == 0 {
		return nil
	}
	out := make([]FieldError, len(e.problems))
	copy(out, e.problems)
	return out
}

func (e *RequestValidationError) FirstMessage() string {
	if e == nil || len(e.problems) == 0 {
		return "Invalid request"
	}
	return e.problems[0].Message
}

type validationBuilder struct {
	problems []FieldError
}

func (b *validationBuilder) add(field, message string) {
	b.problems = append(b.problems, FieldError{
		Field:   field,
		Message: message,
	})
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

	if strings.TrimSpace(req.Token) == "" {
		b.add("token", "Verification token is required")
	}

	return b.err()
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateEmail(field, email string, b *validationBuilder) {
	if email == "" {
		b.add(field, "Email is required")
		return
	}
	if len(email) > maxEmailLength {
		b.add(field, "Email exceeds maximum length")
		return
	}
	if strings.ContainsAny(email, " \t\r\n") {
		b.add(field, "Email must not contain spaces")
		return
	}
	if !utf8.ValidString(email) || hasControlChars(email) {
		b.add(field, "Email contains invalid characters")
		return
	}

	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Address, email) {
		b.add(field, "Email format is invalid")
		return
	}
	if !emailPattern.MatchString(email) {
		b.add(field, "Email format is invalid")
	}
}

func validateCompanyName(name string, b *validationBuilder) {
	if name == "" {
		b.add("company_name", "Company name is required")
		return
	}
	if utf8.RuneCountInString(name) < minCompanyNameLen || utf8.RuneCountInString(name) > maxCompanyNameLen {
		b.add("company_name", "Company name length is invalid")
		return
	}
	if hasControlChars(name) || !labelPattern.MatchString(name) {
		b.add("company_name", "Company name contains invalid characters")
	}
}

func validatePassword(password string, b *validationBuilder) {
	if password == "" {
		b.add("password", "Password is required")
		return
	}
	if strings.TrimSpace(password) != password {
		b.add("password", "Password must not have leading or trailing spaces")
	}
	if strings.ContainsFunc(password, unicode.IsSpace) {
		b.add("password", "Password must not contain whitespace")
	}
	if len(password) < minPasswordLength {
		b.add("password", "Password must be at least 12 characters")
	}
	if len(password) > maxPasswordLength {
		b.add("password", "Password exceeds maximum length")
	}
	if hasControlChars(password) {
		b.add("password", "Password contains invalid characters")
	}

	hasUpper := strings.IndexFunc(password, unicode.IsUpper) >= 0
	hasLower := strings.IndexFunc(password, unicode.IsLower) >= 0
	hasDigit := strings.IndexFunc(password, unicode.IsDigit) >= 0
	hasSpecial := strings.IndexFunc(password, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r)
	}) >= 0

	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		b.add("password", "Password must include upper, lower, number, and special character")
	}
}

func normalizeOptionalText(field, value string, maxLen int, pattern *regexp.Regexp, b *validationBuilder) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if utf8.RuneCountInString(trimmed) > maxLen {
		b.add(field, fmt.Sprintf("%s exceeds maximum length", displayField(field)))
		return ""
	}
	if hasControlChars(trimmed) {
		b.add(field, fmt.Sprintf("%s contains invalid characters", displayField(field)))
		return ""
	}
	if pattern != nil && !pattern.MatchString(trimmed) {
		b.add(field, fmt.Sprintf("%s contains invalid characters", displayField(field)))
		return ""
	}
	return trimmed
}

func normalizeOptionalPointer(field string, value *string, maxLen int, pattern *regexp.Regexp, b *validationBuilder) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	if utf8.RuneCountInString(trimmed) > maxLen {
		b.add(field, fmt.Sprintf("%s exceeds maximum length", displayField(field)))
		return nil
	}
	if hasControlChars(trimmed) {
		b.add(field, fmt.Sprintf("%s contains invalid characters", displayField(field)))
		return nil
	}
	if pattern != nil && !pattern.MatchString(trimmed) {
		b.add(field, fmt.Sprintf("%s contains invalid characters", displayField(field)))
		return nil
	}
	return &trimmed
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

func hasControlChars(v string) bool {
	for _, r := range v {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func displayField(field string) string {
	switch field {
	case "full_name":
		return "Full name"
	case "job_role":
		return "Job role"
	case "industry":
		return "Industry"
	case "use_case":
		return "Use case"
	default:
		return field
	}
}

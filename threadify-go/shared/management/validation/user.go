package validation

import (
	"strings"
	"threadify-go/shared/management/dto"
)

func ValidateUpdateProfileRequest(req *dto.UpdateProfileRequest) error {
	b := &validationBuilder{}
	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}

	req.FullName = strings.TrimSpace(req.FullName)
	if req.FullName == "" {
		b.add("full_name", "Full name is required")
	} else if !validateOptional("full_name", req.FullName, maxFullNameLen, namePattern, b) {
		// already added error in validateOptional
	}

	req.JobRole = strings.TrimSpace(req.JobRole)
	if req.JobRole == "" {
		b.add("job_role", "Job role is required")
	} else if !validateOptional("job_role", req.JobRole, maxJobRoleLen, labelPattern, b) {
		// already added error
	}

	// Company fields are optional (invited users don't need to provide them)
	req.Industry = strings.TrimSpace(req.Industry)
	if req.Industry != "" {
		if !validateOptional("industry", req.Industry, maxIndustryLen, labelPattern, b) {
			// already added error
		}
	}

	req.CompanySize = strings.ToLower(strings.TrimSpace(req.CompanySize))
	if req.CompanySize != "" {
		if _, ok := allowedCompanySizes[req.CompanySize]; !ok {
			b.add("company_size", "Company size must be one of: small, medium, large, enterprise")
		}
	}

	req.UseCase = strings.TrimSpace(req.UseCase)
	if req.UseCase != "" {
		if !validateOptional("use_case", req.UseCase, maxUseCaseLen, nil, b) {
			// already added error
		}
	}

	return b.err()
}

// service/email_templates.go
package service

import "embed"

//go:embed templates/email/*.html
var emailTemplates embed.FS

// Package redisurl parses the Engine's single Redis connection setting.
package redisurl

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// Parse deliberately excludes parser error details, which can contain passwords.
func Parse(raw string) (*redis.Options, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "redis" && u.Scheme != "rediss") || u.Hostname() == "" || u.Opaque != "" {
		return nil, fmt.Errorf("redis.url must be a valid redis:// or rediss:// URL with a host")
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, fmt.Errorf("redis.url does not support query parameters or fragments; use Redis tuning fields separately")
	}
	options, err := redis.ParseURL(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid redis.url credentials, port or database")
	}
	_, port, err := net.SplitHostPort(options.Addr)
	number, numberErr := strconv.Atoi(port)
	if err != nil || numberErr != nil || number < 1 || number > 65535 || options.DB < 0 {
		return nil, fmt.Errorf("redis.url requires a port between 1 and 65535 and a nonnegative database")
	}
	return options, nil
}

// Managed uses the bundled server's TCP listener and default ACL user.
func Managed(raw string) (*redis.Options, error) {
	options, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	if options.TLSConfig != nil {
		return nil, fmt.Errorf("managed Valkey requires redis://; use external mode for rediss:// TLS")
	}
	if options.Username != "" && options.Username != "default" {
		return nil, fmt.Errorf("managed Valkey supports only the default Redis user")
	}
	return options, nil
}

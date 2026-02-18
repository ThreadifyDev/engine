package threadify

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Option configures the connection.
type Option func(*ConnectOptions)

func WithServiceName(name string) Option {
	return func(o *ConnectOptions) {
		o.ServiceName = name
	}
}

func WithWSURL(url string) Option {
	return func(o *ConnectOptions) {
		o.WSURL = url
	}
}

func WithGraphQLURL(url string) Option {
	return func(o *ConnectOptions) {
		o.GraphQLURL = url
	}
}

func WithDebug(debug bool) Option {
	return func(o *ConnectOptions) {
		o.Debug = debug
	}
}

func WithMaxInFlight(max int) Option {
	return func(o *ConnectOptions) {
		o.MaxInFlight = max
	}
}

func WithConnectTimeout(d time.Duration) Option {
	return func(o *ConnectOptions) {
		o.ConnectTimeout = d
	}
}

func WithDialer(d Dialer) Option {
	return func(o *ConnectOptions) {
		o.Dialer = d
	}
}

func Connect(ctx context.Context, apiKey string, opts ...Option) (*Connection, error) {
	if err := requireNonEmpty("apiKey", apiKey); err != nil {
		return nil, err
	}

	o := (&ConnectOptions{}).withDefaults()
	for _, opt := range opts {
		opt(&o)
	}

	if err := o.validate(); err != nil {
		return nil, err
	}

	dialCtx, cancel := context.WithTimeout(ctx, o.ConnectTimeout)
	defer cancel()

	transport, err := o.Dialer.Dial(dialCtx, o.WSURL)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	connectMsg := map[string]any{
		FieldAction:      ActionConnect,
		FieldAPIKey:      apiKey,
		FieldService:     o.ServiceName,
		FieldMaxInFlight: o.MaxInFlight,
	}

	if err := transport.Send(connectMsg); err != nil {
		transport.Close()
		return nil, fmt.Errorf("send connect: %w", err)
	}

	type result struct {
		msg map[string]any
		err error
	}

	ch := make(chan result, 1)
	go func() {
		msg, err := transport.Recv()
		ch <- result{msg, err}
	}()

	select {
	case <-dialCtx.Done():
		transport.Close()
		return nil, fmt.Errorf("connection timeout")
	case r := <-ch:
		if r.err != nil {
			transport.Close()
			return nil, fmt.Errorf("recv connect response: %w", r.err)
		}

		action, _ := r.msg[FieldAction].(string)
		status, _ := r.msg[FieldStatus].(string)

		if action != ActionConnect || status != StatusSuccess {
			transport.Close()
			msg, _ := r.msg[FieldMessage].(string)
			if msg == "" {
				msg = "connection failed"
			}
			return nil, fmt.Errorf("%s", msg)
		}

		conn := newConnection(transport, apiKey, o.ServiceName, o)
		return conn, nil
	}
}

func Create(config Config) *Factory {
	return &Factory{config: config}
}

type Config struct {
	APIKey      string
	ServiceName string
	WSURL       string
	GraphQLURL  string
	Debug       bool
}

type Factory struct {
	config Config
}

func (f *Factory) Connect(ctx context.Context) (*Connection, error) {
	opts := []Option{
		WithServiceName(f.config.ServiceName),
		WithWSURL(f.config.WSURL),
		WithGraphQLURL(f.config.GraphQLURL),
		WithDebug(f.config.Debug),
	}
	return Connect(ctx, f.config.APIKey, opts...)
}

func debugLog(debug bool, format string, args ...any) {
	if debug {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

func asFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}
func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func parseTimestamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}

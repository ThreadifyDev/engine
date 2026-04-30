package scalars

import (
	"encoding/json"
	"io"

	"github.com/99designs/gqlgen/graphql"
)

type JSON interface{}

func MarshalJSON(v JSON) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			_, _ = w.Write([]byte("null\n"))
		}
	})
}

func UnmarshalJSON(v interface{}) (JSON, error) {
	return v, nil
}

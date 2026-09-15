package contractcontent

import (
	"fmt"
	"testing"
)

func TestStepReferences(t *testing.T) {
	ref := Rule{Field: "tracking_number", Operator: "equals", Reference: &Reference{Step: "order_shipped", Field: "tracking_number"}}
	for _, tc := range []struct {
		name, value string
		lookupErr   bool
		ok          bool
	}{{"match", "ABC", false, true}, {"different", "XYZ", false, false}, {"missing source", "ABC", true, false}} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			err := CheckWithReferences([]Rule{ref}, map[string]string{"tracking_number": tc.value}, func(r Reference) (string, error) {
				called = true
				if r.Step != "order_shipped" || r.Field != "tracking_number" {
					t.Fatal(r)
				}
				if tc.lookupErr {
					return "", fmt.Errorf("missing source")
				}
				return "ABC", nil
			})
			if (err == nil) != tc.ok || !called {
				t.Fatalf("err=%v called=%v", err, called)
			}
		})
	}
	if Check([]Rule{ref}, map[string]string{"tracking_number": "ABC"}) == nil {
		t.Fatal("reference accepted without thread resolver")
	}
	literal := Rule{Field: "tracking_number", Operator: "equals", Value: "order_shipped.tracking_number"}
	if err := CheckWithReferences([]Rule{literal}, map[string]string{"tracking_number": "order_shipped.tracking_number"}, func(Reference) (string, error) { t.Fatal("literal resolved as reference"); return "", nil }); err != nil {
		t.Fatal(err)
	}
	for _, rule := range []Rule{{Field: "x", Operator: "number", Reference: ref.Reference}, {Field: "x", Operator: "equals", Value: "literal", Reference: ref.Reference}, {Field: "x", Operator: "equals", Reference: &Reference{Step: "a.b", Field: "x"}}} {
		if Validate([]Rule{rule}) == nil {
			t.Fatal("invalid reference accepted")
		}
	}
}

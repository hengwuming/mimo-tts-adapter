package auth

import "testing"

func TestValidator(t *testing.T) {
	validator := New("secret")
	cases := []struct {
		name   string
		header []string
		want   bool
	}{
		{"valid", []string{"Bearer secret"}, true},
		{"missing", nil, false},
		{"wrong scheme", []string{"Basic secret"}, false},
		{"wrong token", []string{"Bearer other"}, false},
		{"duplicate", []string{"Bearer secret", "Bearer secret"}, false},
		{"extra part", []string{"Bearer secret extra"}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := validator.Authorized(test.header); got != test.want {
				t.Fatalf("Authorized() = %v, want %v", got, test.want)
			}
		})
	}
}

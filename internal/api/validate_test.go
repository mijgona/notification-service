package api

import "testing"

func TestCreateRequestValidate(t *testing.T) {
	cases := []struct {
		name    string
		req     CreateRequest
		wantErr bool
	}{
		{"valid telegram", CreateRequest{IdempotencyKey: "k1", Channel: "telegram", Recipient: "42", Body: "hi"}, false},
		{"valid email", CreateRequest{IdempotencyKey: "k2", Channel: "email", Recipient: "a@b.c", Body: "hi"}, false},
		{"missing idempotency key", CreateRequest{Channel: "email", Recipient: "a@b.c", Body: "hi"}, true},
		{"unknown channel", CreateRequest{IdempotencyKey: "k", Channel: "sms", Recipient: "x", Body: "hi"}, true},
		{"blank body", CreateRequest{IdempotencyKey: "k", Channel: "email", Recipient: "x", Body: "  "}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

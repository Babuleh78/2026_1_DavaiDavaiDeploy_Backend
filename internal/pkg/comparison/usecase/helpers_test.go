package usecase

import (
	"errors"
	"testing"
)

func TestMLServiceURL(t *testing.T) {
	cases := []struct {
		name    string
		baseEnv string
		path    string
		want    string
	}{
		{
			name:    "trims trailing slash on base and leading slash on path",
			baseEnv: "http://ml:8000/",
			path:    "/dance_compare",
			want:    "http://ml:8000/ml/dance_compare",
		},
		{
			name:    "no extra slashes needed",
			baseEnv: "http://ml:8000",
			path:    "process",
			want:    "http://ml:8000/ml/process",
		},
		{
			name:    "empty base still yields a /ml/ path",
			baseEnv: "",
			path:    "status",
			want:    "/ml/status",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ML_SERVICE_URL", tc.baseEnv)
			if got := mlServiceURL(tc.path); got != tc.want {
				t.Errorf("mlServiceURL(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestIsS3NotFoundError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil is not a not-found", err: nil, want: false},
		{name: "NoSuchKey matches", err: errors.New("operation error S3: NoSuchKey: ..."), want: true},
		{name: "StatusCode 404 matches (MinIO)", err: errors.New("https response error StatusCode: 404"), want: true},
		{name: "unrelated error does not match", err: errors.New("connection refused"), want: false},
		// A 403 is an auth problem, not a missing object — must not be swallowed.
		{name: "StatusCode 403 does not match", err: errors.New("https response error StatusCode: 403"), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isS3NotFoundError(tc.err); got != tc.want {
				t.Errorf("isS3NotFoundError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

package usecase

import (
	"errors"
	"testing"

	uuid "github.com/satori/go.uuid"
)

func TestUserInUploaders(t *testing.T) {
	a := uuid.NewV4()
	b := uuid.NewV4()
	c := uuid.NewV4()
	uploaders := []uuid.UUID{a, b}

	t.Run("nil user is never an uploader", func(t *testing.T) {
		if userInUploaders(uploaders, nil) {
			t.Error("nil userID reported as uploader")
		}
	})
	t.Run("member found", func(t *testing.T) {
		if !userInUploaders(uploaders, &a) {
			t.Error("known uploader not found")
		}
	})
	t.Run("non-member not found", func(t *testing.T) {
		if userInUploaders(uploaders, &c) {
			t.Error("non-uploader reported as uploader")
		}
	})
	t.Run("empty uploader list", func(t *testing.T) {
		if userInUploaders(nil, &a) {
			t.Error("matched against empty uploader list")
		}
	})
}

func TestNormalizeDanceDifficulty(t *testing.T) {
	cases := []struct {
		label     string
		wantLabel string
		wantScore int
	}{
		{"easy", "easy", 25},
		{"hard", "hard", 75},
		{"medium", "medium", 50},
		// Anything unrecognised (including empty) falls back to medium.
		{"", "medium", 50},
		{"EASY", "medium", 50},
		{"expert", "medium", 50},
	}

	for _, tc := range cases {
		t.Run("label="+tc.label, func(t *testing.T) {
			label, score := normalizeDanceDifficulty(tc.label)
			if label != tc.wantLabel || score != tc.wantScore {
				t.Errorf("normalizeDanceDifficulty(%q) = (%q, %d), want (%q, %d)",
					tc.label, label, score, tc.wantLabel, tc.wantScore)
			}
		})
	}
}

func TestMLServiceURL(t *testing.T) {
	t.Setenv("ML_SERVICE_URL", "http://ml:8000/")
	if got := mlServiceURL("/moderate"); got != "http://ml:8000/ml/moderate" {
		t.Errorf("mlServiceURL = %q, want http://ml:8000/ml/moderate", got)
	}
}

func TestIsS3NotFoundError(t *testing.T) {
	if isS3NotFoundError(nil) {
		t.Error("nil error reported as not-found")
	}
	if !isS3NotFoundError(errors.New("NoSuchKey")) {
		t.Error("NoSuchKey not recognised")
	}
	if !isS3NotFoundError(errors.New("StatusCode: 404")) {
		t.Error("StatusCode: 404 not recognised")
	}
	if isS3NotFoundError(errors.New("timeout")) {
		t.Error("unrelated error reported as not-found")
	}
}

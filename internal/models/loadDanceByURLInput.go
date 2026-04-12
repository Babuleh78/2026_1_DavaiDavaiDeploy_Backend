package models
import "strings"	
type LoadDanceByURLInput struct {
    URL    string `json:"url"`
}

func (l *LoadDanceByURLInput) Sanitize() {
    l.URL = strings.TrimSpace(l.URL)
}
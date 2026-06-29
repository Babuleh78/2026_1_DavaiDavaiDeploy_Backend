package helpers

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, data interface{}) {
	body, err := json.Marshal(data)
	if err != nil {
		WriteError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(body)
}

func WriteError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": status,
		"error":  http.StatusText(status),
	})
}

func WriteXML(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	err := xml.NewEncoder(w).Encode(data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

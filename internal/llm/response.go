package llm

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeOpenAIError(w http.ResponseWriter, status int, message, errorType, param string) {
	var paramPtr *string
	if param != "" {
		paramPtr = &param
	}

	writeJSON(w, status, errorResponse{
		Error: errorBody{
			Message: message,
			Type:    errorType,
			Param:   paramPtr,
			Code:    nil,
		},
	})
}

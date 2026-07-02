package concurrency

import (
	"encoding/json"
	"net/http"
)

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    *string `json:"code"`
}

func writeConcurrencyError(w http.ResponseWriter, status int, message, errorType string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error: errorBody{
			Message: message,
			Type:    errorType,
			Param:   nil,
			Code:    nil,
		},
	})
}

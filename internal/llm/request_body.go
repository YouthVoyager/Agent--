package llm

import "encoding/json"

func rewriteRequestModel(rawBody []byte, model string) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return nil, err
	}

	modelValue, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	payload["model"] = modelValue
	return json.Marshal(payload)
}

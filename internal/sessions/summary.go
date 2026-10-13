package sessions

import "encoding/json"

// List summaries need message identity and call relationships, but not command
// arguments or outputs. Project them while decoding so large outputs are skipped
// by encoding/json instead of copied into RawMessage and decoded again.
type summaryPayload struct {
	Type        json.RawMessage `json:"type,omitempty"`
	Role        json.RawMessage `json:"role,omitempty"`
	ID          json.RawMessage `json:"id,omitempty"`
	CWD         json.RawMessage `json:"cwd,omitempty"`
	Timestamp   json.RawMessage `json:"timestamp,omitempty"`
	HistoryBase json.RawMessage `json:"history_base,omitempty"`
	Model       json.RawMessage `json:"model,omitempty"`
	TurnID      json.RawMessage `json:"turn_id,omitempty"`
	CallID      json.RawMessage `json:"call_id,omitempty"`
	Name        json.RawMessage `json:"name,omitempty"`
	Message     json.RawMessage `json:"message,omitempty"`
	Content     json.RawMessage `json:"content,omitempty"`
	Item        *summaryPayload `json:"item,omitempty"`
	raw         json.RawMessage
}

func (p *summaryPayload) UnmarshalJSON(data []byte) error {
	type fields summaryPayload
	err := json.Unmarshal(data, (*fields)(p))
	if _, ok := err.(*json.UnmarshalTypeError); ok {
		p.raw = append(p.raw[:0], data...)
		return nil
	}
	return err
}

func decodeSummaryRecord(data []byte) (record, error) {
	var projected struct {
		Timestamp string          `json:"timestamp"`
		Type      string          `json:"type"`
		Payload   *summaryPayload `json:"payload"`
	}
	if err := json.Unmarshal(data, &projected); err != nil {
		return record{}, err
	}
	r := record{Timestamp: projected.Timestamp, Type: projected.Type}
	if projected.Payload == nil {
		return r, nil
	}
	p := projected.Payload
	if p.raw != nil {
		r.Payload = p.raw
		return r, nil
	}
	r.fields = map[string]json.RawMessage{
		"type": p.Type, "role": p.Role, "id": p.ID, "cwd": p.CWD,
		"timestamp": p.Timestamp, "history_base": p.HistoryBase,
		"model": p.Model, "turn_id": p.TurnID, "call_id": p.CallID,
		"name": p.Name, "message": p.Message, "content": p.Content,
	}
	if p.Item != nil {
		if p.Item.raw != nil {
			r.fields["item"] = p.Item.raw
		} else {
			item, err := json.Marshal(p.Item)
			if err != nil {
				return r, err
			}
			r.fields["item"] = item
		}
	}
	return r, nil
}

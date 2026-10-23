package redis

import (
	"encoding/json"

	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/backend/history"
)

type eventWithoutAttributes struct {
	*history.Event
}

func (e *eventWithoutAttributes) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		*history.Event
		Attributes interface{} `json:"attr"`
	}{
		Event:      e.Event,
		Attributes: nil,
	})
}

func marshalEventWithoutAttributes(event *history.Event) (string, error) {
	data, err := json.Marshal(&eventWithoutAttributes{event})
	if err != nil {
		return "", ctxerrors.Wrap(err, "execute events")
	}

	return string(data), nil
}

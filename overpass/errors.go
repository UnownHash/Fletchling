package overpass

import (
	"bytes"
	"errors"
	"fmt"
)

var (
	errTimeout   = errors.New("timeout occurred")
	errDupeQuery = errors.New("dupe query")

	readAndIdxBytes     = []byte("Dispatcher_Client::request_read_and_idx::")
	errReadAndIdxTokens = []struct {
		token []byte
		err   error
	}{
		{[]byte("timeout"), errTimeout},
		{[]byte("duplicate_query"), errDupeQuery},
	}
)

func matchBodyAgainstErrors(body []byte) error {
	idx := bytes.Index(body, readAndIdxBytes)
	if idx >= 0 {
		body_at_token := body[idx+len(readAndIdxBytes):]
		l := len(body_at_token)
		for _, entry := range errReadAndIdxTokens {
			token_len := len(entry.token)
			if token_len > l {
				continue
			}
			token := body_at_token[:token_len]
			if bytes.Equal(token, entry.token) {
				return entry.err
			}
		}
		return fmt.Errorf("unknown error: %s", string(body_at_token))
	}
	return nil
}

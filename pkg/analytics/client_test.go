package analytics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestAnalytics_Hash(t *testing.T) {
	userId := "klotho@example.com"

	cases := []struct {
		name   string
		given  any
		expect string
	}{
		{
			name:  "string",
			given: "hello",
			// printf '%s\n' 'klotho@example.com"hello"' | sha256sum
			// Note that json.Marshal always adds a newline to the end of output
			expect: "sha256:0a5261c4c416db5ecea3b865596c9c8cc6ff2a84697bbb9a689154e372b55649",
		},
		{
			name:  "number",
			given: 123,
			// printf '%s\n' 'klotho@example.com123' | sha256sum
			expect: "sha256:eb70010f1d91932a75a80f0edf4717bd39e841a174608261c68ed87bb0f73dc2",
		},
		{
			name:  "bytes",
			given: []byte{1, 2, 3},
			// printf '\x01\x02\x03' | base64  ==> AQID
			// printf '%s\n' 'klotho@example.com"AQID"' | sha256sum
			expect: "sha256:ce7bae11139f0ed51b5f7b74cb773146a542d771e235440e7e2022a0be52f892",
		},
		{
			name:  "nil",
			given: nil,
			// printf '%s\n' 'klotho@example.comnull' | sha256sum
			expect: "sha256:35f7637a859d9e720d7c9736d0d90cafe23ecddecfb977e93b2c9830f91f4ff4",
		},
		{
			name:  "jsonable object",
			given: jsonConvertable{Foo: "bar"},
			// printf '%s\n' 'klotho@example.com{"foo":"bar"}' | sha256sum
			expect: "sha256:926ed2b2760419cf62f7c2ade5c75a20a26cf4ba9d32119fd7dc045ae9333a12",
		},
		{
			name:   "not jsonable object",
			given:  func() {},
			expect: "unknown",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			analytics := &Client{
				userId: userId,
			}
			actual := analytics.Hash(tt.given)
			assert.Equal(tt.expect, actual)
		})
	}
}

func TestAnalyticsSend(t *testing.T) {
	cases := []struct {
		name   string
		send   func(client *Client)
		expect []sentPayload
	}{
		{
			name: "direct send at level info with properties",
			send: func(c *Client) {
				c.userId = "my-user@klo.dev"
				c.AppendProperty("property_1", "aaa")
				c.Info("hello world")
			},
			expect: []sentPayload{{
				"id":    "my-user@klo.dev",
				"event": "hello world",
				"properties": map[string]any{
					"_logLevel":  "info",
					"status":     "info",
					"property_1": "aaa",
				},
			}},
		},
		{
			name: "two sends have isolated state",
			send: func(c *Client) {
				c.userId = "my-user@klo.dev"

				payload1 := c.createPayload(Warn, "message 1")
				payload1.Properties["hello"] = "world"

				payload2 := c.createPayload(Info, "message 2")
				payload2.Properties["hello"] = "goodbye"

				c.send(payload1)
				c.send(payload2)
			},
			expect: []sentPayload{
				{
					"id":    "my-user@klo.dev",
					"event": "message 1",
					"properties": map[string]any{
						"_logLevel": "warn",
						"status":    "warn",
						"hello":     "world",
					},
				},
				{
					"id":    "my-user@klo.dev",
					"event": "message 2",
					"properties": map[string]any{
						"_logLevel": "info",
						"status":    "info",
						"hello":     "goodbye",
					},
				},
			},
		},
		{
			name: "send via logger",
			send: func(c *Client) {
				c.userId = "my-user@klo.dev"
				logger := zap.New(c.NewFieldListener(zapcore.WarnLevel))
				logger.Error("first message", zap.Error(fmt.Errorf("my error")))
				logger.Warn("second message") // no error field on this one!
			},
			expect: []sentPayload{
				{
					"id":    "my-user@klo.dev",
					"event": "ERROR",
					"properties": map[string]any{
						"_logLevel": "error",
						"status":    "error",
						"error":     "my error",
					},
				},
				{
					"id":    "my-user@klo.dev",
					"event": "WARN",
					"properties": map[string]any{
						"_logLevel": "warn",
						"status":    "warn",
					},
				},
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			handler := interactions{assert: assert}
			for range tt.expect {
				handler.interactions = append(handler.interactions, nil)
			}

			server := httptest.NewServer(&handler)
			defer server.Close()

			// Don't use NewClient which uses local state
			// thus causes the test to depend on the runner's environment
			client := &Client{
				serverUrlOverride: server.URL,
				universalProperties: map[string]any{
					"localId": "localId",
				},
			}

			tt.send(client)
			for i, receivedPayload := range handler.interactions {
				expect := tt.expect[i]
				for k, v := range client.universalProperties {
					expect["properties"].(map[string]any)[k] = v
				}
				if assert.NotNil(receivedPayload) {

					assert.Equal(expect, receivedPayload)
				}
			}
		})
	}
}

type (
	sentPayload     map[string]any
	jsonConvertable struct {
		Foo string `json:"foo"`
	}

	interactions struct {
		assert       *assert.Assertions
		count        int
		interactions []sentPayload
	}
)

func (s *interactions) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if s.count >= len(s.interactions) {
		s.assert.Fail("no interactions left")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() { s.count += 1 }()

	decoder := json.NewDecoder(r.Body)
	body := sentPayload{}
	if err := decoder.Decode(&body); !s.assert.NoError(err) {
		return
	}
	s.interactions[s.count] = body

	if s.assert.Equal(http.MethodPost, r.Method) && s.assert.Equal("/analytics/track", r.URL.RequestURI()) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("ok"))
		s.assert.NoError(err)
	} else {
		s.assert.Fail("no interactions left")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

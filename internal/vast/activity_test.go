package vast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstanceDecodesProviderStatusMessage(t *testing.T) {
	for _, message := range []string{`"9b18e2ebedf4: Verifying Checksum"`, "null"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"instances":{"id":987,"actual_status":"loading","status_msg":` + message + `}}`))
		}))
		instance, err := NewClient(server.URL, "token").GetInstance(context.Background(), 987)
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if message != "null" && instance.StatusMessage != "9b18e2ebedf4: Verifying Checksum" {
			t.Fatalf("status message lost: %+v", instance)
		}
		if message == "null" && instance.StatusMessage != "" {
			t.Fatal("null status must be empty")
		}
	}
}

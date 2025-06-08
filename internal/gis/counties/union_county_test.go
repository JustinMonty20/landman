package counties

import (
  "net/http/httptest"
  "net/http"
  "fmt"
)

func mockCountSuccess(count int) *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(fmt.Sprintf(`{"count": %d}`, count)))
    }))
}
